package skills

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

type AuditSeverity string

const (
	AuditSeverityError   AuditSeverity = "error"
	AuditSeverityWarning AuditSeverity = "warning"
)

type AuditFinding struct {
	Severity AuditSeverity `json:"severity"`
	Rule     string        `json:"rule"`
	Location string        `json:"location,omitempty"`
	Message  string        `json:"message"`
}

type AuditReport struct {
	Findings []AuditFinding `json:"findings,omitempty"`
}

type AuditError struct {
	Findings []AuditFinding
}

func (err *AuditError) Error() string {
	if err == nil || len(err.Findings) == 0 {
		return ""
	}

	parts := make([]string, 0, len(err.Findings))
	for _, finding := range err.Findings {
		if strings.TrimSpace(finding.Location) != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", finding.Location, finding.Message))
			continue
		}
		parts = append(parts, finding.Message)
	}
	return "static audit failed: " + strings.Join(parts, "; ")
}

func (report *AuditReport) HasErrors() bool {
	if report == nil {
		return false
	}
	for _, finding := range report.Findings {
		if finding.Severity == AuditSeverityError {
			return true
		}
	}
	return false
}

func (report *AuditReport) WarningCount() int {
	if report == nil {
		return 0
	}
	count := 0
	for _, finding := range report.Findings {
		if finding.Severity == AuditSeverityWarning {
			count++
		}
	}
	return count
}

func (report *AuditReport) BlockingError() error {
	if report == nil {
		return nil
	}
	findings := make([]AuditFinding, 0)
	for _, finding := range report.Findings {
		if finding.Severity != AuditSeverityError {
			continue
		}
		findings = append(findings, finding)
	}
	if len(findings) == 0 {
		return nil
	}
	return &AuditError{Findings: findings}
}

func AuditDir(dir string) (*AuditReport, error) {
	asset, err := LoadAssetDir(dir)
	if err != nil {
		return nil, err
	}
	return auditLoadedAsset(dir, asset)
}

func auditLoadedAsset(baseDir string, asset *models.Asset) (*AuditReport, error) {
	report := &AuditReport{}
	auditHookCommand(report, baseDir, "hooks.post_install", asset.Manifest.Hooks.PostInstall)
	auditHookCommand(report, baseDir, "hooks.post_pull", asset.Manifest.Hooks.PostPull)
	if err := auditPackageFiles(report, baseDir); err != nil {
		return nil, err
	}
	if len(report.Findings) == 0 {
		return nil, nil
	}
	return report, nil
}

func auditHookCommand(report *AuditReport, baseDir, location string, command *models.AssetCommand) {
	if report == nil || command == nil {
		return
	}
	if len(command.Run) == 0 {
		report.addFinding(AuditSeverityError, "empty-hook-command", location, "hook command is empty")
		return
	}

	launcher := strings.TrimSpace(command.Run[0])
	launcherBase := strings.ToLower(filepath.Base(launcher))
	if filepath.IsAbs(launcher) {
		report.addFinding(AuditSeverityWarning, "host-absolute-command", location, fmt.Sprintf("hook uses host absolute command path %q", launcher))
	}
	if isRelativeExecutablePath(launcher) {
		if err := requirePackageFile(baseDir, launcher, location+".run[0]"); err != nil {
			report.addFinding(AuditSeverityError, "missing-hook-command", location, err.Error())
		}
	}
	if isInlineShellCommand(launcherBase, command.Run[1:]) {
		report.addFinding(AuditSeverityError, "inline-shell-command", location, "hook uses inline shell execution; check in a script file instead")
		return
	}
	if scriptPath := hookScriptPath(command.Run); scriptPath != "" {
		if err := requirePackageFile(baseDir, scriptPath, location+".script"); err != nil {
			report.addFinding(AuditSeverityError, "missing-hook-script", location, err.Error())
		}
	}
	if isNetworkHook(command.Run) {
		report.addFinding(AuditSeverityWarning, "network-access-hook", location, fmt.Sprintf("hook invokes network-capable command %q; review for hermeticity and supply-chain risk", launcherBase))
	}
}

func auditPackageFiles(report *AuditReport, baseDir string) error {
	files, err := collectPackageFiles(baseDir)
	if err != nil {
		return fmt.Errorf("collect package files for audit: %w", err)
	}
	for _, file := range files {
		if !looksSensitiveFile(file.RelativePath) {
			continue
		}
		report.addFinding(AuditSeverityWarning, "sensitive-file", file.RelativePath, "package includes a file name that commonly carries credentials or private keys")
	}
	return nil
}

func (report *AuditReport) addFinding(severity AuditSeverity, rule, location, message string) {
	if report == nil {
		return
	}
	report.Findings = append(report.Findings, AuditFinding{
		Severity: severity,
		Rule:     rule,
		Location: location,
		Message:  message,
	})
}

func isRelativeExecutablePath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "-") {
		return false
	}
	return strings.Contains(trimmed, "/") || strings.HasPrefix(trimmed, ".")
}

func isInlineShellCommand(launcher string, args []string) bool {
	if !isShellLauncher(launcher) {
		return false
	}
	for _, arg := range args {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "-c", "/c", "-command", "-encodedcommand":
			return true
		}
	}
	return false
}

func isShellLauncher(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sh", "bash", "zsh", "fish", "ksh", "cmd", "powershell", "pwsh":
		return true
	default:
		return false
	}
}

func hookScriptPath(run []string) string {
	if len(run) == 0 {
		return ""
	}
	if isRelativeExecutablePath(run[0]) {
		return strings.TrimSpace(run[0])
	}
	if !isShellLauncher(filepath.Base(run[0])) {
		return ""
	}
	for _, arg := range run[1:] {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}
		if isRelativeExecutablePath(trimmed) {
			return trimmed
		}
		break
	}
	return ""
}

func isNetworkHook(run []string) bool {
	if len(run) == 0 {
		return false
	}
	launcher := strings.ToLower(strings.TrimSpace(filepath.Base(run[0])))
	switch launcher {
	case "curl", "wget", "ftp", "scp", "sftp", "ssh", "nc", "ncat", "telnet":
		return true
	case "powershell", "pwsh":
		for _, arg := range run[1:] {
			value := strings.ToLower(strings.TrimSpace(arg))
			if value == "invoke-webrequest" || value == "iwr" || value == "irm" {
				return true
			}
		}
	}
	return false
}

func looksSensitiveFile(relativePath string) bool {
	base := strings.ToLower(filepath.Base(relativePath))
	switch base {
	case ".env", ".env.local", ".env.production", ".netrc", ".npmrc", ".pypirc", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	default:
		return false
	}
}
