// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ByteSize mirrors cmdyaml.ByteSize / cmdtypes.ByteSize (TextMarshaler/TextUnmarshaler).
type ByteSize int64

func (b ByteSize) MarshalText() ([]byte, error) {
	const mb = 1_000_000
	if b > 0 && b%mb == 0 {
		return fmt.Appendf(nil, "%dMB", b/mb), nil
	}
	return []byte(strconv.FormatInt(int64(b), 10)), nil
}

func (b *ByteSize) UnmarshalText(text []byte) error {
	s := strings.TrimSpace(string(text))
	if strings.HasSuffix(strings.ToUpper(s), "MB") {
		num := strings.TrimSpace(s[:len(s)-2])
		n, err := strconv.ParseInt(num, 10, 64)
		if err != nil {
			return err
		}
		*b = ByteSize(n * 1_000_000)
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*b = ByteSize(n)
	return nil
}

// LogFormat models a flags.Enum[T]-style enum wrapper.
type LogFormat string

func (LogFormat) EnumValues() map[string]LogFormat {
	return map[string]LogFormat{
		"text": "text",
		"json": "json",
	}
}

type Enum[T interface {
	comparable
	EnumValues() map[string]T
}] struct {
	Value T
}

func (e Enum[T]) String() string {
	for k, v := range e.Value.EnumValues() {
		if v == e.Value {
			return k
		}
	}
	return fmt.Sprintf("%v", e.Value)
}

func (e Enum[T]) MarshalText() ([]byte, error) {
	return []byte(e.String()), nil
}

func (e *Enum[T]) UnmarshalText(text []byte) error {
	v, ok := e.Value.EnumValues()[string(text)]
	if !ok {
		return fmt.Errorf("invalid enum value %q", string(text))
	}
	e.Value = v
	return nil
}

// KeychainType mirrors keychain.Type (EnumValues directly on an int enum).
type KeychainType int

const (
	KeychainFileBased KeychainType = iota + 1
	KeychainDataProtectionLocal
	KeychainICloud
	KeychainAll
)

func (KeychainType) EnumValues() map[string]KeychainType {
	return map[string]KeychainType{
		"file":                  KeychainFileBased,
		"data-protection-local": KeychainDataProtectionLocal,
		"icloud":                KeychainICloud,
		"all":                   KeychainAll,
	}
}

// KeychainAccessibility mirrors keychain.Accessibility.
type KeychainAccessibility int

const (
	AccessibleDefault KeychainAccessibility = iota
	AccessibleWhenUnlocked
	AccessibleAfterFirstUnlock
	AccessibleAlways
)

func (KeychainAccessibility) EnumValues() map[string]KeychainAccessibility {
	return map[string]KeychainAccessibility{
		"default":            AccessibleDefault,
		"when-unlocked":      AccessibleWhenUnlocked,
		"after-first-unlock": AccessibleAfterFirstUnlock,
		"always":             AccessibleAlways,
	}
}

// StagingBehaviour mirrors vmspool.StagingBehaviour.
type StagingBehaviour int

const (
	StagingBehaviourRunning StagingBehaviour = iota
	StagingBehaviourSuspended
	StagingBehaviourStopped
)

func (StagingBehaviour) EnumValues() map[string]StagingBehaviour {
	return map[string]StagingBehaviour{
		"Running":   StagingBehaviourRunning,
		"Suspended": StagingBehaviourSuspended,
		"Stopped":   StagingBehaviourStopped,
	}
}

func (s StagingBehaviour) MarshalText() ([]byte, error) {
	switch s {
	case StagingBehaviourRunning:
		return []byte("Running"), nil
	case StagingBehaviourSuspended:
		return []byte("Suspended"), nil
	case StagingBehaviourStopped:
		return []byte("Stopped"), nil
	default:
		return nil, fmt.Errorf("unknown staging behaviour %d", s)
	}
}

func (s *StagingBehaviour) UnmarshalText(b []byte) error {
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case "running":
		*s = StagingBehaviourRunning
	case "suspended":
		*s = StagingBehaviourSuspended
	case "stopped":
		*s = StagingBehaviourStopped
	default:
		return fmt.Errorf("unknown StagingBehaviour %q", string(b))
	}
	return nil
}

// BaseLoggingConfig mirrors cmdutil.LoggingConfig.
type BaseLoggingConfig struct {
	Level      int             `yaml:"level" doc:"logging level: 0=error, 1=warn, 2=info, 3=debug"`
	File       string          `yaml:"file" doc:"log file path. If not specified logs are written to stderr."`
	Format     Enum[LogFormat] `yaml:"format" doc:"log format: text or json"`
	SourceCode bool            `yaml:"source_code" doc:"include source code file and line number in logs"`
}

type LoggingConfig struct {
	BaseLoggingConfig `yaml:",inline"`
	MaxSize           ByteSize `yaml:"max_size" doc:"maximum size of a log file before it is rotated; defaults to 10MB"`
	MaxBackups        int      `yaml:"max_backups" doc:"maximum number of rotated log files to retain; defaults to 5"`
}

type GlobalConfig struct {
	TmpDir                      string        `yaml:"tmp_dir" doc:"directory in which to create temporary files and directories"`
	SearchPaths                 []string      `yaml:"search_paths" doc:"list of paths to search for required tools (e.g. tart etc.)" tview:"widget=csv"`
	CompletionQueueSize         int           `yaml:"completion_queue_size" doc:"maximum number of completion events to retain in memory"`
	FailedVMRetentionPeriod     time.Duration `yaml:"failed_vm_retention_period" doc:"duration for which failed VMs should be retained before being deleted"`
	SuccessfulVMRetentionPeriod time.Duration `yaml:"successful_vm_retention_period" doc:"duration for which successful VMs should be retained before being deleted"`
}

type KeySpec struct {
	User  string `yaml:"user" doc:"keychain user/account"`
	KeyID string `yaml:"key_id" doc:"identifier of the key in the key store"`
}

type GitHubServiceConfig struct {
	Owner   string `yaml:"owner" doc:"GitHub repository owner or organization"`
	Repo    string `yaml:"repo" doc:"GitHub repository name"`
	PerPage int    `yaml:"per_page" doc:"API pagination size"`
	UserID  string `yaml:"user_id" doc:"keychain user ID for GitHub token"`
	KeyID   string `yaml:"key_id" doc:"key ID for GitHub runner token"`
}

type RunnerConfig struct {
	NamePrefix string        `yaml:"name_prefix" doc:"prefix for name of the runner"`
	VMPoolName string        `yaml:"vm_pool" doc:"name of the VM pool to use for this runner"`
	Labels     []string      `yaml:"labels,flow" doc:"labels to assign to the runner" tview:"widget=csv"`
	Replace    bool          `yaml:"replace" doc:"if true, replace any existing runner with the same name"`
	Ephemeral  bool          `yaml:"ephemeral" doc:"if true, register the runner as ephemeral"`
	Timeout    time.Duration `yaml:"timeout" doc:"maximum time to wait for the runner to complete a job"`
}

func (rc *RunnerConfig) SetDefaults() {
	rc.NamePrefix = "runner"
	rc.VMPoolName = "macos"
	rc.Labels = []string{"self-hosted"}
	rc.Ephemeral = true
	rc.Timeout = 30 * time.Minute
}

func (rc RunnerConfig) Validate() error {
	if rc.NamePrefix == "" {
		return fmt.Errorf("runner missing name_prefix")
	}
	if len(rc.Labels) == 0 {
		return fmt.Errorf("runner %s: at least one label is required", rc.NamePrefix)
	}
	if rc.VMPoolName == "" {
		return fmt.Errorf("runner %s: missing vm_pool", rc.NamePrefix)
	}
	return nil
}

type RepositoryConfig struct {
	GitHubServiceConfig `yaml:",inline"`
	Runners             []RunnerConfig `yaml:"runners" doc:"list of runner configurations for this repository" tview:"itemlabel=NamePrefix"`
}

func (rc *RepositoryConfig) SetDefaults() {
	rc.Owner = "cloudengio"
	rc.Repo = "new-repo"
	rc.PerPage = 100
	rc.Runners = []RunnerConfig{{
		NamePrefix: "go-macos",
		VMPoolName: "macos",
		Labels:     []string{"self-hosted", "macos", "arm64"},
		Replace:    true,
		Ephemeral:  true,
		Timeout:    30 * time.Minute,
	}}
}

func (rc RepositoryConfig) Validate() error {
	if rc.Owner == "" || rc.Repo == "" {
		return fmt.Errorf("repository requires owner and repo")
	}
	if len(rc.Runners) == 0 {
		return fmt.Errorf("repository %s: at least one runner configuration is required", rc.Repo)
	}
	for _, r := range rc.Runners {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("repository %s: %w", rc.Repo, err)
		}
	}
	return nil
}

type KeychainPluginConfig struct {
	Binary        string                `yaml:"plugin_binary" doc:"plugin binary to use"`
	Type          KeychainType          `yaml:"keychain_type" doc:"the type of keychain to use"`
	Account       string                `yaml:"account" doc:"account that the keychain item belongs to"`
	UpdateInPlace bool                  `yaml:"update_in_place" doc:"set to true to update existing item in place"`
	Accessibility KeychainAccessibility `yaml:"accessibility" doc:"optional accessibility level for the keychain item"`
	Direct        bool                  `yaml:"-"`
}

type ICloudKeychainConfig struct {
	KeychainPluginConfig `yaml:",inline"`
	Items                []string `yaml:"items" doc:"list of keychain item names to use for API keys"`
}

type ExponentialBackoffConfig struct {
	InitialDelay time.Duration `yaml:"initial_delay" doc:"initial backoff delay"`
	Steps        int           `yaml:"steps" doc:"maximum number of exponential backoff steps"`
	StatusCodes  []int         `yaml:"status_codes" doc:"HTTP status codes triggering retry"`
}

type BasePoolConfig struct {
	Size                  int                      `yaml:"size" doc:"number of VMs to maintain in the pool"`
	CleanupTimeout        time.Duration            `yaml:"cleanup_timeout" doc:"timeout for cleaning up VMs"`
	CreateBackoff         ExponentialBackoffConfig `yaml:"create_backoff" doc:"backoff applied between attempts to create a VM"`
	StopTimeout           time.Duration            `yaml:"stop_timeout" doc:"timeout for stopping VMs"`
	StagingBehaviour      StagingBehaviour         `yaml:"staging_behaviour" doc:"staging behaviour for VMs in the pool"`
	DeleteAcquiredOnClose bool                     `yaml:"delete_acquired_on_close" doc:"controls whether Close deletes acquired VMs"`
}

type Signer struct {
	Key    string `yaml:"key" doc:"PEM-encoded cosign public key" tview:"widget=textarea"`
	Issuer string `yaml:"issuer" doc:"OIDC issuer for keyless verification"`
	Email  string `yaml:"email" doc:"signer email/subject for keyless verification"`
}

type VerificationPolicy struct {
	Signers         []Signer          `yaml:"signers" doc:"trusted signers" tview:"itemlabel=Email"`
	TransparencyLog bool              `yaml:"require_transparency_log" doc:"require transparency log entry"`
	Annotations     map[string]string `yaml:"annotations" doc:"required signature annotations"`
}

type OCISource struct {
	Image           string              `yaml:"image" doc:"OCI image reference to create VMs from"`
	PullRemoteOnRun bool                `yaml:"pull_remote_on_run" doc:"pull image from registry before creating each VM"`
	Insecure        bool                `yaml:"pull_insecure" doc:"allow pulling image without TLS verification"`
	Concurrency     int                 `yaml:"pull_concurrency" doc:"concurrency used to pull image"`
	Verify          *VerificationPolicy `yaml:"verify" doc:"optional cosign verification policy"`
}

type TartVMConfig struct {
	OS               string                   `yaml:"os" doc:"operating system of the tart VM" tview:"widget=dropdown,options=macos|linux"`
	StateBackoff     ExponentialBackoffConfig `yaml:"state_backoff" doc:"backoff when polling VM state"`
	RunBackoff       ExponentialBackoffConfig `yaml:"run_backoff" doc:"backoff when waiting for VM to run"`
	ForceStopBackoff ExponentialBackoffConfig `yaml:"force_stop_backoff" doc:"backoff when forcefully stopping a VM"`
	RunOptions       []string                 `yaml:"run_options,flow" doc:"additional options to pass to tart run" tview:"widget=csv"`
	TartBinary       string                   `yaml:"binary" doc:"path to the tart binary"`
}

type VMResources struct {
	CPUs     int      `yaml:"cpus" doc:"virtual CPUs allocated per VM"`
	Memory   ByteSize `yaml:"memory" doc:"memory allocated per VM"`
	DiskSize ByteSize `yaml:"disk_size" doc:"disk size allocated per VM"`
}

type TartConfig struct {
	TartVMConfig `yaml:",inline"`
	OCISource    `yaml:",inline"`
	VMResources  `yaml:",inline"`
	RunnerDir    string `yaml:"runner_dir" doc:"directory on the VM in which the runner is installed"`
	CosignBinary string `yaml:"cosign_binary" doc:"path to cosign binary"`
}

type MockConfig struct {
	RunnerDir   string        `yaml:"runner_dir" doc:"mock guest runner directory"`
	StartupWait time.Duration `yaml:"startup_wait" doc:"simulated VM startup latency"`
}

type PoolConfig struct {
	BasePoolConfig `yaml:",inline"`
	Tart           *TartConfig `yaml:"tart_config" doc:"configure and select the tart VM backend for this pool"`
	Mock           *MockConfig `yaml:"mock_config" doc:"configure and select the mock VM backend for this pool"`
	Backend        any         `yaml:"-"`
}

func (p PoolConfig) Validate() error {
	if p.Tart != nil && p.Mock != nil {
		return fmt.Errorf("more than one VM backend configured (set only one of tart_config or mock_config)")
	}
	if p.Tart == nil && p.Mock == nil {
		return fmt.Errorf("no VM backend configured (set tart_config or mock_config)")
	}
	if p.Tart != nil && p.Tart.Image == "" {
		return fmt.Errorf("tart_config.image is required")
	}
	return nil
}

type RateControlConfig struct {
	ExponentialBackoff ExponentialBackoffConfig `yaml:"exponential_backoff" doc:"exponential backoff configuration"`
}

type WebhookConfig struct {
	RelayURL    string            `yaml:"relay_url" doc:"the URL to which GitHub will send webhook events"`
	RateControl RateControlConfig `yaml:"rate_control" doc:"rate control settings for the webhook relay service"`
	Streaming   bool              `yaml:"streaming" doc:"use the streaming webhook API instead of polling"`
}

type CookieScopeAndDuration struct {
	Domain   string        `yaml:"domain" doc:"cookie domain"`
	Path     string        `yaml:"path" doc:"cookie path"`
	Duration time.Duration `yaml:"duration" doc:"cookie lifetime"`
}

type JWTCookieConfig struct {
	Name                   string `yaml:"name" doc:"name of the authentication cookie"`
	CookieScopeAndDuration `yaml:",inline"`
	Insecure               bool `yaml:"insecure" doc:"allow insecure (non-HTTPS) cookie"`
}

type JWTSignerConfig struct {
	Issuer   string            `yaml:"jwt_issuer" doc:"jwt issuer"`
	Audience []string          `yaml:"jwt_audience" doc:"jwt audience" tview:"widget=csv"`
	Duration time.Duration     `yaml:"jwt_duration" doc:"validity duration of the issued JWT"`
	Subject  string            `yaml:"jwt_subject" doc:"jwt subject"`
	Claims   map[string]string `yaml:"jwt_claims" doc:"additional claims to include in issued JWTs"`
}

type JWTValidatorConfig struct {
	JWTCookieConfig    `yaml:"cookie" doc:"jwt cookie config"`
	ValidationTimeSkew time.Duration     `yaml:"validation_time_skew" doc:"allowed time skew for validation"`
	Issuer             string            `yaml:"jwt_issuer" doc:"expected jwt issuer"`
	Audience           []string          `yaml:"jwt_audience" doc:"expected jwt audience" tview:"widget=csv"`
	Subject            string            `yaml:"jwt_subject" doc:"expected jwt subject"`
	Claims             map[string]string `yaml:"jwt_claims" doc:"additional expected claims"`
	VerificationKeys   []KeySpec         `yaml:"jwt_verification_keys" doc:"jwt verification key specs" tview:"itemlabel=KeyID"`
}

type JWTIssuerConfig struct {
	JWTCookieConfig `yaml:"cookie" doc:"jwt cookie config"`
	JWTSignerConfig `yaml:",inline"`
	SigningKey      KeySpec `yaml:"jwt_signing_key" doc:"jwt signing key spec"`
	Redirect        string  `yaml:"redirect" doc:"URL to redirect to after setting the cookie"`
}

type WebAssetsConfig struct {
	ReloadEnabled bool   `yaml:"reload_enabled" doc:"serve web UI assets from local filesystem"`
	ReloadRoot    string `yaml:"reload_root" doc:"path to webui directory for live reload"`
}

type WebUIConfig struct {
	Enabled         bool                `yaml:"enabled" doc:"enable the web UI and management API"`
	ListenAddress   string              `yaml:"listen_address" doc:"address for the web UI/API HTTP server"`
	Reload          WebAssetsConfig     `yaml:"reload" doc:"local asset reload settings"`
	JWTVerification *JWTValidatorConfig `yaml:"jwt_verifier" doc:"configuration for JWT verification"`
}

type Config struct {
	Logging        LoggingConfig         `yaml:"logging" doc:"logging configuration"`
	Global         GlobalConfig          `yaml:"global" doc:"global configuration options"`
	Repositories   []RepositoryConfig    `yaml:"repositories" doc:"list of GitHub repositories to manage runners for" tview:"itemlabel=Repo"`
	ICloudKeychain ICloudKeychainConfig  `yaml:"icloud_keychain" doc:"icloud keychain configuration for loading API keys"`
	VMPools        map[string]PoolConfig `yaml:"vm_pools" doc:"configuration for VM pools to use for runner provisioning" tview:"expand"`
	Webhook        WebhookConfig         `yaml:"webhook" doc:"configuration for the GitHub webhook relay service"`
	WebUI          WebUIConfig           `yaml:"web_ui" doc:"configuration for the management web UI and JSON API"`
	JWTIssuer      *JWTIssuerConfig      `yaml:"jwt_issuer" doc:"configuration for the JWT issuer used by the orchestrator"`
}

func (cfg Config) Validate() error {
	if cfg.WebUI.Enabled && cfg.WebUI.ListenAddress != "" {
		host, _, err := net.SplitHostPort(cfg.WebUI.ListenAddress)
		if err != nil {
			host = cfg.WebUI.ListenAddress
		}
		if host != "127.0.0.1" && host != "localhost" {
			return fmt.Errorf("web_ui.listen_address must bind to loopback, got %q", cfg.WebUI.ListenAddress)
		}
	}
	for _, repo := range cfg.Repositories {
		if err := repo.Validate(); err != nil {
			return err
		}
		for _, r := range repo.Runners {
			if _, ok := cfg.VMPools[r.VMPoolName]; !ok {
				return fmt.Errorf("repository %s: runner %s references unknown vm_pool %q", repo.Repo, r.NamePrefix, r.VMPoolName)
			}
		}
	}
	for name, pool := range cfg.VMPools {
		if err := pool.Validate(); err != nil {
			return fmt.Errorf("vm_pool %s: %w", name, err)
		}
	}
	return nil
}

func DefaultConfig() Config {
	cookie := JWTCookieConfig{
		Name: "github-runner-orchestrator",
		CookieScopeAndDuration: CookieScopeAndDuration{
			Domain:   "127.0.0.1",
			Path:     "/",
			Duration: 240 * time.Hour,
		},
		Insecure: true,
	}
	runners := []RunnerConfig{
		{
			NamePrefix: "go-macos",
			VMPoolName: "macos",
			Labels:     []string{"self-hosted", "macos", "arm64"},
			Replace:    true,
			Ephemeral:  true,
			Timeout:    30 * time.Minute,
		},
		{
			NamePrefix: "go-linux",
			VMPoolName: "linux",
			Labels:     []string{"self-hosted", "linux", "arm64"},
			Replace:    true,
			Ephemeral:  true,
			Timeout:    30 * time.Minute,
		},
	}

	return Config{
		Logging: LoggingConfig{
			BaseLoggingConfig: BaseLoggingConfig{
				Level:  2,
				File:   "-",
				Format: Enum[LogFormat]{Value: "text"},
			},
			MaxSize:    10_000_000,
			MaxBackups: 5,
		},
		Global: GlobalConfig{
			TmpDir:                      "github-orchestrator-*",
			SearchPaths:                 []string{"/opt/homebrew/bin", "/usr/local/bin"},
			CompletionQueueSize:         1000,
			FailedVMRetentionPeriod:     time.Hour,
			SuccessfulVMRetentionPeriod: 15 * time.Minute,
		},
		ICloudKeychain: ICloudKeychainConfig{
			KeychainPluginConfig: KeychainPluginConfig{
				Type:          KeychainAll,
				Accessibility: AccessibleWhenUnlocked,
			},
			Items: []string{
				"cloudengio-github-runner",
				"webhook-secrets",
				"cloudengio-jwt-signing-keys",
			},
		},
		VMPools: map[string]PoolConfig{
			"macos": {
				BasePoolConfig: BasePoolConfig{
					Size:                  2,
					CleanupTimeout:        time.Minute,
					StopTimeout:           30 * time.Second,
					StagingBehaviour:      StagingBehaviourRunning,
					DeleteAcquiredOnClose: true,
					CreateBackoff:         ExponentialBackoffConfig{InitialDelay: 500 * time.Millisecond, Steps: 10},
				},
				Tart: &TartConfig{
					TartVMConfig: TartVMConfig{
						OS:         "macos",
						TartBinary: "/opt/homebrew/bin/tart",
					},
					OCISource: OCISource{
						Image:    "mm-1:5001/macos-ci:latest",
						Insecure: true,
						Verify: &VerificationPolicy{
							Signers: []Signer{{Key: "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----", Email: "ci@cloudeng.io"}},
						},
					},
					VMResources:  VMResources{CPUs: 4, Memory: 8_000_000_000},
					RunnerDir:    "/Users/admin/actions-runner",
					CosignBinary: "/opt/homebrew/bin/cosign",
				},
			},
			"linux": {
				BasePoolConfig: BasePoolConfig{
					Size:                  2,
					StagingBehaviour:      StagingBehaviourStopped,
					DeleteAcquiredOnClose: true,
				},
				Tart: &TartConfig{
					TartVMConfig: TartVMConfig{
						OS:         "linux",
						RunOptions: []string{"--no-graphics", "--no-audio", "--nested"},
						TartBinary: "/opt/homebrew/bin/tart",
					},
					OCISource: OCISource{
						Image:    "mm-1:5001/linux-ci:latest",
						Insecure: true,
					},
					RunnerDir: "/home/admin/actions-runner",
				},
			},
		},
		Repositories: []RepositoryConfig{
			{
				GitHubServiceConfig: GitHubServiceConfig{
					Owner:   "cloudengio",
					Repo:    "go.pkgs",
					PerPage: 100,
					UserID:  "cnicolaou",
					KeyID:   "github-cloudengio-runner-token",
				},
				Runners: runners,
			},
			{
				GitHubServiceConfig: GitHubServiceConfig{
					Owner:   "cloudengio",
					Repo:    "citools",
					PerPage: 100,
					UserID:  "cnicolaou",
					KeyID:   "github-cloudengio-runner-token",
				},
				Runners: runners,
			},
		},
		Webhook: WebhookConfig{
			RelayURL: "https://example.invalid:10444/webhooks/github_workflows/relay",
			RateControl: RateControlConfig{
				ExponentialBackoff: ExponentialBackoffConfig{
					InitialDelay: time.Second,
					Steps:        10,
					StatusCodes:  []int{429, 502, 503},
				},
			},
		},
		WebUI: WebUIConfig{
			Enabled:       true,
			ListenAddress: "127.0.0.1:8089",
			JWTVerification: &JWTValidatorConfig{
				JWTCookieConfig:    cookie,
				ValidationTimeSkew: time.Minute,
				Issuer:             "github-orchestrator",
				Audience:           []string{"github-orchestrator"},
				Subject:            "github-orchestrator",
				Claims:             map[string]string{"access": "admin"},
				VerificationKeys:   []KeySpec{{User: "cnicolaou", KeyID: "github-orchestrator-jwt-key"}},
			},
		},
		JWTIssuer: &JWTIssuerConfig{
			JWTCookieConfig: cookie,
			JWTSignerConfig: JWTSignerConfig{
				Issuer:   "github-orchestrator",
				Audience: []string{"github-orchestrator"},
				Duration: 24 * time.Hour,
				Subject:  "github-orchestrator",
				Claims:   map[string]string{"access": "admin"},
			},
			SigningKey: KeySpec{User: "cnicolaou", KeyID: "github-orchestrator-jwt-key"},
		},
	}
}
