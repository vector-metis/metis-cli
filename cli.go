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
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	contract "github.com/vector-metis/metis-sdk-contracts"
)

// ManifestMountFact 是 inspect 输出的 manifest 挂载投影，便于开发者核对最终 scope 相对路径。
type ManifestMountFact struct {
	Service  string `json:"service"`
	Source   string `json:"source"`
	Subpath  string `json:"subpath,omitempty"`
	Rendered string `json:"rendered"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

// ImageVolumeMatch 是 inspect 输出的镜像声明 volume 与 manifest target 的匹配事实。
type ImageVolumeMatch struct {
	Architecture string `json:"architecture"`
	Service      string `json:"service"`
	Image        string `json:"image"`
	Volume       string `json:"volume"`
	Source       string `json:"source,omitempty"`
	Subpath      string `json:"subpath,omitempty"`
	Matched      bool   `json:"matched"`
}

// PreflightSetting is the safe setting projection used by CI reports.
// Secret defaults are represented only by DefaultSet.
type PreflightSetting struct {
	Key          string `json:"key"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	DefaultSet   bool   `json:"defaultSet"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

// PreflightReport is a machine-readable package preflight report.
type PreflightReport struct {
	OK            bool               `json:"ok"`
	ID            string             `json:"id"`
	Version       string             `json:"version"`
	SHA256        string             `json:"sha256"`
	Size          int64              `json:"size"`
	Architectures []string           `json:"architectures"`
	Capabilities  []string           `json:"capabilities"`
	ModelSlots    []string           `json:"modelSlots"`
	Settings      []PreflightSetting `json:"settings"`
	Checks        []string           `json:"checks"`
}

// SimulationSelection is one automatically selected or manually overridden resource.
type SimulationSelection struct {
	Mode  string `json:"mode"`
	Value string `json:"value,omitempty"`
}

// SimulationSetting is a safe setting projection; secret values are redacted.
type SimulationSetting struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Value  string `json:"value,omitempty"`
}

// SimulationReport is a local install or upgrade plan for developer review.
type SimulationReport struct {
	Operation        string                         `json:"operation"`
	AppID            string                         `json:"appId"`
	Version          string                         `json:"version"`
	Architecture     SimulationSelection            `json:"architecture"`
	Node             SimulationSelection            `json:"node"`
	Models           map[string]SimulationSelection `json:"models"`
	SharedBuckets    []SimulationSelection          `json:"sharedBuckets"`
	Settings         []SimulationSetting            `json:"settings"`
	PreserveExisting bool                           `json:"preserveExistingSettings"`
	PreflightChecks  []string                       `json:"preflightChecks"`
}

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
	root.AddCommand(newInitCommand(), newPackCommand(), newValidateCommand(), newPreflightCommand(), newInspectCommand(), newSimulateCommand())
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

func newPreflightCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "preflight <工作目录或 .mpk>",
		Short: "在上传或安装前执行本地 manifest 预检",
		Long:  "复用平台 MPK 门禁校验 settings 默认值、模型声明、Compose 和镜像事实；--json 输出可供 CI 使用的报告。平台资源可用性仍需由平台安装预检确认。",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			summary, cleanup, err := validatePath(args[0])
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				return err
			}
			report := makePreflightReport(*summary)
			if asJSON {
				encoder := json.NewEncoder(command.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}
			if _, err = fmt.Fprintf(command.OutOrStdout(), "预检通过\t%s/%s\t%s\t%d bytes\n", report.ID, report.Version, report.SHA256, report.Size); err != nil {
				return err
			}
			for _, setting := range report.Settings {
				defaultState := "已声明"
				if !setting.DefaultSet {
					defaultState = "缺失"
				}
				if _, err = fmt.Fprintf(command.OutOrStdout(), "  setting %-24s default=%s required=%t\n", setting.Key, defaultState, setting.Required); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "输出机器可读 JSON 报告")
	return command
}

func makePreflightReport(summary contract.PackageSummary) PreflightReport {
	settings := make([]PreflightSetting, 0, len(summary.Manifest.Settings))
	for _, setting := range summary.Manifest.Settings {
		item := PreflightSetting{Key: setting.Key, Type: setting.Type, Required: setting.Required, DefaultSet: setting.DefaultSet}
		if setting.Type != "secret" {
			item.DefaultValue = setting.Default
		}
		settings = append(settings, item)
	}
	return PreflightReport{
		OK: true, ID: summary.Manifest.ID, Version: summary.Manifest.Version,
		SHA256: summary.SHA256, Size: summary.Size,
		Architectures: append([]string(nil), summary.Manifest.Architectures...),
		Capabilities:  append([]string(nil), summary.Manifest.Capabilities...),
		ModelSlots:    contract.ModelSlotsFromManifest(summary.Manifest), Settings: settings,
		Checks: []string{"manifest schema 与身份", "settings default/type/options/range", "service、模型 slot、挂载和 Compose", "镜像归档与声明一致性"},
	}
}

func newSimulateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "simulate",
		Short: "模拟应用安装或升级计划",
		Long:  "读取工作目录或 MPK 并生成本地计划；节点、模型和共享存储默认标记为平台自动选择，资源可用性仍需由平台预检确认。",
	}
	command.AddCommand(newSimulationOperationCommand("install"), newSimulationOperationCommand("upgrade"))
	return command
}

func newSimulationOperationCommand(operation string) *cobra.Command {
	var architecture string
	var nodeID int64
	var modelOverrides []string
	var settingOverrides []string
	var sharedBuckets []string
	var asJSON bool
	command := &cobra.Command{
		Use:   operation + " <工作目录或 .mpk>",
		Short: "模拟" + operationLabel(operation) + "计划",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			report, err := buildSimulationReport(args[0], operation, architecture, nodeID, modelOverrides, settingOverrides, sharedBuckets)
			if err != nil {
				return err
			}
			if asJSON {
				encoder := json.NewEncoder(command.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}
			return writeSimulationReport(command.OutOrStdout(), report)
		},
	}
	command.Flags().StringVar(&architecture, "arch", "", "目标架构；省略时使用 manifest 的第一个架构")
	command.Flags().Int64Var(&nodeID, "node-id", 0, "覆盖节点 ID；省略时由平台自动选择")
	command.Flags().StringArrayVar(&modelOverrides, "model", nil, "覆盖模型 slot，格式为 slot=model，可重复")
	command.Flags().StringArrayVar(&settingOverrides, "setting", nil, "覆盖 setting，格式为 key=value，可重复")
	command.Flags().StringArrayVar(&sharedBuckets, "shared-bucket", nil, "覆盖共享桶标识，可重复；省略时由平台自动选择")
	command.Flags().BoolVar(&asJSON, "json", false, "输出机器可读 JSON")
	return command
}

func operationLabel(operation string) string {
	if operation == "upgrade" {
		return "升级"
	}
	return "安装"
}

func buildSimulationReport(input, operation, architecture string, nodeID int64, modelOverrides, settingOverrides, sharedBuckets []string) (SimulationReport, error) {
	if operation != "install" && operation != "upgrade" {
		return SimulationReport{}, fmt.Errorf("不支持的模拟操作 %q", operation)
	}
	summary, cleanup, err := validatePath(input)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return SimulationReport{}, err
	}
	if len(summary.Manifest.Architectures) == 0 {
		return SimulationReport{}, errors.New("manifest.arch 不能为空")
	}
	if architecture == "" {
		architecture = summary.Manifest.Architectures[0]
	}
	if !slices.Contains(summary.Manifest.Architectures, architecture) {
		return SimulationReport{}, fmt.Errorf("架构 %q 不在 manifest.arch 中", architecture)
	}
	settings, err := parseSimulationSettings(summary.Manifest.Settings, settingOverrides)
	if err != nil {
		return SimulationReport{}, err
	}
	models, err := parseSimulationModels(summary.Manifest.Models, modelOverrides)
	if err != nil {
		return SimulationReport{}, err
	}
	buckets := make([]SimulationSelection, 0, len(sharedBuckets))
	for _, value := range sharedBuckets {
		value = strings.TrimSpace(value)
		if value == "" {
			return SimulationReport{}, errors.New("--shared-bucket 不能是空值")
		}
		buckets = append(buckets, SimulationSelection{Mode: "override", Value: value})
	}
	if len(buckets) == 0 {
		buckets = []SimulationSelection{{Mode: "auto"}}
	}
	nodeSelection := SimulationSelection{Mode: "auto"}
	if nodeID != 0 {
		if nodeID < 0 {
			return SimulationReport{}, errors.New("--node-id 必须大于 0")
		}
		nodeSelection = SimulationSelection{Mode: "override", Value: fmt.Sprintf("%d", nodeID)}
	}
	return SimulationReport{
		Operation: operation, AppID: summary.Manifest.ID, Version: summary.Manifest.Version,
		Architecture: SimulationSelection{Mode: "selected", Value: architecture}, Node: nodeSelection,
		Models: models, SharedBuckets: buckets, Settings: settings,
		PreserveExisting: operation == "upgrade", PreflightChecks: []string{
			"manifest schema、settings default/type/options/range 已通过",
			"service、模型 slot、挂载、Compose 和镜像事实已通过",
		},
	}, nil
}

func parseSimulationSettings(definitions []contract.Setting, overrides []string) ([]SimulationSetting, error) {
	values := make(map[string]string, len(overrides))
	for _, item := range overrides {
		key, value, ok := strings.Cut(item, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" {
			return nil, fmt.Errorf("--setting 必须使用 key=value 格式：%q", item)
		}
		values[key] = value
	}
	if err := contract.ValidateSettings(definitions, values); err != nil {
		return nil, err
	}
	result := make([]SimulationSetting, 0, len(definitions))
	for _, definition := range definitions {
		value, overridden := values[definition.Key]
		source := "default"
		if overridden {
			source = "override"
		} else {
			value = definition.Default
		}
		if definition.Type == "secret" {
			value = "[redacted]"
		}
		result = append(result, SimulationSetting{Key: definition.Key, Source: source, Value: value})
	}
	return result, nil
}

func parseSimulationModels(definitions map[string]contract.ModelSlot, overrides []string) (map[string]SimulationSelection, error) {
	result := make(map[string]SimulationSelection, len(definitions))
	for _, slot := range contract.ModelSlotsFromManifest(contract.Manifest{Models: definitions}) {
		result[slot] = SimulationSelection{Mode: "auto"}
	}
	for _, item := range overrides {
		slot, value, ok := strings.Cut(item, "=")
		slot, value = strings.TrimSpace(slot), strings.TrimSpace(value)
		if !ok || slot == "" || value == "" {
			return nil, fmt.Errorf("--model 必须使用 slot=model 格式：%q", item)
		}
		if _, exists := definitions[slot]; !exists {
			return nil, fmt.Errorf("模型 slot %q 未在 manifest.models 中声明", slot)
		}
		result[slot] = SimulationSelection{Mode: "override", Value: value}
	}
	return result, nil
}

func writeSimulationReport(output io.Writer, report SimulationReport) error {
	if _, err := fmt.Fprintf(output, "模拟%s：%s/%s\n", operationLabel(report.Operation), report.AppID, report.Version); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "架构：%s\n节点：%s\n", report.Architecture.Value, selectionText(report.Node)); err != nil {
		return err
	}
	if len(report.Models) == 0 {
		if _, err := fmt.Fprintln(output, "模型：无模型 slot"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(output, "模型："); err != nil {
			return err
		}
		for _, slot := range sortedSimulationKeys(report.Models) {
			if _, err := fmt.Fprintf(output, "  %s：%s\n", slot, selectionText(report.Models[slot])); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(output, "共享存储："); err != nil {
		return err
	}
	for _, bucket := range report.SharedBuckets {
		if _, err := fmt.Fprintf(output, "  %s\n", selectionText(bucket)); err != nil {
			return err
		}
	}
	for _, setting := range report.Settings {
		if _, err := fmt.Fprintf(output, "设置 %s：%s（%s）\n", setting.Key, setting.Value, setting.Source); err != nil {
			return err
		}
	}
	if report.PreserveExisting {
		_, _ = fmt.Fprintln(output, "升级规则：同名 setting 保留现值，新增 setting 使用 manifest 默认值")
	}
	return nil
}

func selectionText(selection SimulationSelection) string {
	if selection.Mode == "auto" {
		return "平台自动选择"
	}
	if selection.Value == "" {
		return selection.Mode
	}
	return selection.Value + "（手动覆盖）"
}

func sortedSimulationKeys(values map[string]SimulationSelection) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
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
			var imageVolumeMatches []ImageVolumeMatch
			if !infoIsDirectory(args[0]) {
				images, imageSummaries, imageVolumeMatches, err = inspectPackageImageFacts(args[0], summary.Manifest)
				if err != nil {
					return err
				}
			}
			mounts := manifestMountFacts(summary.Manifest)
			encoder := json.NewEncoder(command.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(struct {
				Manifest       contract.Manifest                        `json:"manifest"`
				Size           int64                                    `json:"size"`
				SHA256         string                                   `json:"sha256"`
				FileCount      int                                      `json:"fileCount"`
				Images         []contract.PackageImage                  `json:"images,omitempty"`
				ImageSummaries map[string]*contract.ImageArchiveSummary `json:"imageSummaries,omitempty"`
				Mounts         []ManifestMountFact                      `json:"mounts,omitempty"`
				ImageVolumes   []ImageVolumeMatch                       `json:"imageVolumeMatches,omitempty"`
			}{summary.Manifest, summary.Size, summary.SHA256, summary.FileCount, images, imageSummaries, mounts, imageVolumeMatches})
		},
	}
	return command
}

func infoIsDirectory(input string) bool {
	info, err := os.Stat(input)
	return err == nil && info.IsDir()
}

func inspectPackageImageFacts(input string, manifest contract.Manifest) ([]contract.PackageImage, map[string]*contract.ImageArchiveSummary, []ImageVolumeMatch, error) {
	file, err := os.Open(input)
	if err != nil {
		return nil, nil, nil, err
	}
	defer file.Close()
	metadata, err := contract.InspectPackageMetadata(file, manifest.ID, manifest.Version)
	if err != nil {
		return nil, nil, nil, err
	}
	summaries := make(map[string]*contract.ImageArchiveSummary)
	images, err := contract.InspectPackageImages(file, *metadata, func(archive contract.PackageImageArchive) (*contract.ImageArchiveSummary, error) {
		summary, inspectErr := contract.InspectImageArchiveStream(archive.Body)
		if inspectErr != nil {
			return nil, inspectErr
		}
		summaries[archive.Path] = summary
		return summary, nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	volumeMatches, err := imageVolumeMatches(metadata, images, summaries)
	if err != nil {
		return nil, nil, nil, err
	}
	return images, summaries, volumeMatches, nil
}

func manifestMountFacts(manifest contract.Manifest) []ManifestMountFact {
	result := make([]ManifestMountFact, 0)
	for serviceName, service := range manifest.Services {
		for _, mount := range service.Mounts {
			rendered := "./" + mount.Source
			if mount.Subpath != "" {
				rendered += "/" + mount.Subpath
			}
			result = append(result, ManifestMountFact{
				Service: serviceName, Source: mount.Source, Subpath: mount.Subpath,
				Rendered: rendered, Target: mount.Target, ReadOnly: mount.ReadOnly,
			})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Service != result[right].Service {
			return result[left].Service < result[right].Service
		}
		return result[left].Target < result[right].Target
	})
	return result
}

func imageVolumeMatches(metadata *contract.PackageMetadata, images []contract.PackageImage, summaries map[string]*contract.ImageArchiveSummary) ([]ImageVolumeMatch, error) {
	bindings, err := contract.ImageVolumeBindings(*metadata, images, summaries)
	if err != nil {
		return nil, err
	}
	result := make([]ImageVolumeMatch, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, ImageVolumeMatch{
			Architecture: binding.Architecture, Service: binding.Service, Image: binding.Image,
			Volume: binding.Volume, Source: binding.Source, Subpath: binding.Subpath, Matched: true,
		})
	}
	return result, nil
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
	manifest := fmt.Sprintf("schema_version: 1\nid: %s\nversion: 0.1.0\ndisplay_name: %s\ndescription: \"\"\ntype: web\narch: [%s]\ndependencies: []\nsettings: {}\nmodels: {}\nservices:\n  web:\n    lifecycle: {restart: unless-stopped}\n    endpoints:\n      - {name: web, protocol: http, container_port: 8080}\n", appID, appID, archYAML)
	files := map[string]string{
		"manifest.yaml": manifest,
		"about.md":      "# " + appID + "\n",
	}
	for _, architecture := range architectures {
		files["compose."+architecture+".yaml"] = fmt.Sprintf("services:\n  web:\n    image: %s/web:0.1.0\n", appID)
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
