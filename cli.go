// Package cli 提供 metis 开发者命令行工具的可测试命令树。
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	contract "github.com/vector-metis/metis-sdk-contracts"
)

// NewCommand 创建新的命令树；每次测试都应调用本函数，避免 Cobra flag 状态串扰。
func NewCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "metis",
		Short:         "构建和校验 Metis 应用安装包",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.AddCommand(newInitCommand(), newPackCommand(), newValidateCommand(), newInspectCommand())
	return root
}

func newInitCommand() *cobra.Command {
	var appID string
	var architectures []string
	command := &cobra.Command{
		Use:   "init [目录]",
		Short: "初始化应用工作目录",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			if appID == "" {
				absolute, err := filepath.Abs(target)
				if err != nil {
					return fmt.Errorf("解析目标目录: %w", err)
				}
				appID = filepath.Base(absolute)
			}
			return initWorkspace(target, appID, architectures, command.OutOrStdout())
		},
	}
	command.Flags().StringVar(&appID, "id", "", "应用标识；默认使用目标目录名")
	command.Flags().StringSliceVar(&architectures, "arch", []string{contract.ArchAMD64}, "目标架构：amd64,arm64")
	return command
}

func newPackCommand() *cobra.Command {
	var output string
	var noValidate bool
	command := &cobra.Command{
		Use:   "pack <工作目录>",
		Short: "打包成不可变 .mpk",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if output == "" {
				return errors.New("必须使用 --output 指定 .mpk 输出路径")
			}
			return packWorkspace(args[0], output, !noValidate, command.OutOrStdout())
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "输出 .mpk 文件")
	command.Flags().BoolVar(&noValidate, "no-validate", false, "跳过打包后的本地校验")
	return command
}

func newValidateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "validate <工作目录或 .mpk>",
		Short: "执行完整本地包校验",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			summary, cleanup, err := validatePath(args[0])
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "校验通过\t%s\t%s/%s\t%d bytes\n", summary.SHA256, summary.Manifest.ID, summary.Manifest.Version, summary.Size)
			return err
		},
	}
	return command
}

func newInspectCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "inspect <工作目录或 .mpk>",
		Short: "输出包元数据",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.EqualFold(filepath.Ext(args[0]), ".tar") {
				file, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("打开镜像归档: %w", err)
				}
				defer file.Close()
				summary, err := contract.InspectImageArchive(file)
				if err != nil {
					return err
				}
				encoder := json.NewEncoder(command.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(summary)
			}
			summary, cleanup, err := validatePath(args[0])
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				return err
			}
			var images []contract.PackageImage
			imageSummaries := map[string]*contract.ImageArchiveSummary{}
			if !infoIsDirectory(args[0]) {
				images, imageSummaries, err = inspectPackageImageFacts(args[0], summary.Manifest)
				if err != nil {
					return err
				}
			}
			encoder := json.NewEncoder(command.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(struct {
				Manifest       contract.Manifest                        `json:"manifest"`
				Size           int64                                    `json:"size"`
				SHA256         string                                   `json:"sha256"`
				FileCount      int                                      `json:"fileCount"`
				Images         []contract.PackageImage                  `json:"images,omitempty"`
				ImageSummaries map[string]*contract.ImageArchiveSummary `json:"imageSummaries,omitempty"`
			}{summary.Manifest, summary.Size, summary.SHA256, summary.FileCount, images, imageSummaries})
		},
	}
	return command
}

func infoIsDirectory(input string) bool {
	info, err := os.Stat(input)
	return err == nil && info.IsDir()
}

func inspectPackageImageFacts(input string, manifest contract.Manifest) ([]contract.PackageImage, map[string]*contract.ImageArchiveSummary, error) {
	file, err := os.Open(input)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	metadata, err := contract.InspectPackageMetadata(file, manifest.ID, manifest.Version)
	if err != nil {
		return nil, nil, err
	}
	summaries := make(map[string]*contract.ImageArchiveSummary)
	images, err := contract.InspectPackageImages(file, *metadata, func(archive contract.PackageImageArchive) (*contract.ImageArchiveSummary, error) {
		temporary, createErr := os.CreateTemp("", "metis-inspect-image-*.tar")
		if createErr != nil {
			return nil, createErr
		}
		name := temporary.Name()
		defer os.Remove(name)
		if _, copyErr := io.Copy(temporary, archive.Body); copyErr != nil {
			_ = temporary.Close()
			return nil, copyErr
		}
		if _, seekErr := temporary.Seek(0, io.SeekStart); seekErr != nil {
			_ = temporary.Close()
			return nil, seekErr
		}
		summary, inspectErr := contract.InspectImageArchive(temporary)
		closeErr := temporary.Close()
		if inspectErr != nil || closeErr != nil {
			return nil, errors.Join(inspectErr, closeErr)
		}
		summaries[archive.Path] = summary
		return summary, nil
	})
	return images, summaries, err
}

func initWorkspace(target, appID string, architectures []string, output io.Writer) error {
	architectures, err := normalizeArchitectures(architectures)
	if err != nil {
		return err
	}
	if len(architectures) == 0 {
		return errors.New("至少需要一个架构：amd64 或 arm64")
	}
	if appID == "" || strings.ContainsAny(appID, `/\\`) {
		return errors.New("应用标识不能为空且不能包含路径分隔符")
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("创建工作目录: %w", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("读取工作目录: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("目标目录 %q 必须为空", target)
	}
	directories := []string{"icons", "overlay"}
	for _, architecture := range architectures {
		directories = append(directories, filepath.Join("images", architecture))
	}
	for _, directory := range directories {
		if err := os.MkdirAll(filepath.Join(target, directory), 0o755); err != nil {
			return fmt.Errorf("创建目录 %q: %w", directory, err)
		}
	}
	archYAML := strings.Join(architectures, ", ")
	manifest := fmt.Sprintf("schema_version: 1\nid: %s\nversion: 0.1.0\ndisplay_name: %s\ndescription: \"\"\ntype: web\narch: [%s]\ndependencies: []\nservices:\n  web:\n    endpoints:\n      - {name: web, protocol: http, container_port: 8080}\n", appID, appID, archYAML)
	files := map[string]string{
		"manifest.yaml": manifest,
		"about.md":      "# " + appID + "\n",
	}
	for _, architecture := range architectures {
		files["compose."+architecture+".yaml"] = fmt.Sprintf("services:\n  web:\n    image: %s/web:0.1.0\n    restart: unless-stopped\n", appID)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(target, name), []byte(content), 0o644); err != nil {
			return fmt.Errorf("写入 %q: %w", name, err)
		}
	}
	_, err = fmt.Fprintf(output, "已初始化 %s（%s）\n", target, strings.Join(architectures, ","))
	return err
}

func normalizeArchitectures(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			architecture := strings.ToLower(strings.TrimSpace(item))
			if architecture != contract.ArchAMD64 && architecture != contract.ArchARM64 {
				return nil, fmt.Errorf("不支持的架构 %q；只允许 amd64 或 arm64", architecture)
			}
			if _, exists := seen[architecture]; exists {
				continue
			}
			seen[architecture] = struct{}{}
			result = append(result, architecture)
		}
	}
	sort.Strings(result)
	return result, nil
}

func packWorkspace(source, outputPath string, validate bool, output io.Writer) error {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	if relative, relErr := filepath.Rel(absSource, absOutput); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("输出文件不能位于待打包目录内")
	}
	if err := os.MkdirAll(filepath.Dir(absOutput), 0o755); err != nil {
		return fmt.Errorf("创建输出目录: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(absOutput), ".metis-pack-*.mpk")
	if err != nil {
		return fmt.Errorf("创建临时包: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = contract.BuildMPK(absSource, temporary); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return fmt.Errorf("同步临时包: %w", err)
	}
	if validate {
		if _, err = temporary.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err = contract.GateMPK(temporary, contract.GateOptions{}); err != nil {
			return err
		}
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporaryPath, absOutput); err != nil {
		return fmt.Errorf("提交输出包: %w", err)
	}
	committed = true
	_, err = fmt.Fprintf(output, "%s\n", outputPath)
	return err
}

func validatePath(input string) (*contract.PackageSummary, func(), error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, nil, err
	}
	path := input
	cleanup := func() {}
	if info.IsDir() {
		var buffer bytes.Buffer
		if err := contract.BuildMPK(input, &buffer); err != nil {
			return nil, nil, err
		}
		return validateReader(bytes.NewReader(buffer.Bytes()), cleanup)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	cleanup = func() { _ = file.Close() }
	return validateReader(file, cleanup)
}

func validateReader(reader io.ReadSeeker, cleanup func()) (*contract.PackageSummary, func(), error) {
	gate, err := contract.GateMPK(reader, contract.GateOptions{})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return &contract.PackageSummary{
		Manifest:  gate.Metadata.Manifest,
		Size:      gate.Size,
		SHA256:    gate.SHA256,
		AboutMD:   gate.Metadata.AboutMD,
		FileCount: gate.Metadata.FileCount,
	}, cleanup, nil
}
