// Copyright 2026 cloudeng llc. All rights reserved.
// Use of this source code is governed by the Apache-2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"time"

	"cloudeng.io/algo/ratecontrol"
	"cloudeng.io/cmdutil"
	"cloudeng.io/cmdutil/cmdyaml"
	"cloudeng.io/cmdutil/flags"
	"cloudeng.io/cmdutil/keys"
	"cloudeng.io/file/crawl/crawlcmd"
	"cloudeng.io/vms/ociimage"
	basepool "cloudeng.io/vms/vmspool"
	"cloudeng.io/webapp/cookies"
	"cloudeng.io/webapp/webassets"
	"cloudeng.io/webapp/webauth/jwtutil"
)

// LoggingConfig matches orchestrator.LoggingConfig using cloudeng.io/cmdutil.
type LoggingConfig struct {
	cmdutil.LoggingConfig `yaml:",inline"`
	MaxSize               cmdyaml.ByteSize `yaml:"max_size" doc:"maximum size of a log file before it is rotated; defaults to 10MB"`
	MaxBackups            int              `yaml:"max_backups" doc:"maximum number of rotated log files to retain; defaults to 5"`
}

type GlobalConfig struct {
	TmpDir                      string        `yaml:"tmp_dir" doc:"directory in which to create temporary files and directories"`
	SearchPaths                 []string      `yaml:"search_paths" doc:"list of paths to search for required tools (e.g. tart etc.)" tview:"widget=csv"`
	CompletionQueueSize         int           `yaml:"completion_queue_size" doc:"maximum number of completion events to retain in memory"`
	FailedVMRetentionPeriod     time.Duration `yaml:"failed_vm_retention_period" doc:"duration for which failed VMs should be retained before being deleted"`
	SuccessfulVMRetentionPeriod time.Duration `yaml:"successful_vm_retention_period" doc:"duration for which successful VMs should be retained before being deleted"`
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

// KeychainType demonstrates flags.Enum[T] from cloudeng.io/cmdutil/flags.
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

// KeychainAccessibility demonstrates a bare EnumValues type.
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

type KeychainPluginConfig struct {
	Binary        string                   `yaml:"plugin_binary" doc:"plugin binary to use"`
	Type          flags.Enum[KeychainType] `yaml:"keychain_type" doc:"the type of keychain to use"`
	Account       string                   `yaml:"account" doc:"account that the keychain item belongs to"`
	UpdateInPlace bool                     `yaml:"update_in_place" doc:"set to true to update existing item in place"`
	Accessibility KeychainAccessibility    `yaml:"accessibility" doc:"optional accessibility level for the keychain item"`
	Direct        bool                     `yaml:"-"`
}

type ICloudKeychainConfig struct {
	KeychainPluginConfig `yaml:",inline"`
	Items                []string `yaml:"items" doc:"list of keychain item names to use for API keys"`
}

type TartVMConfig struct {
	OS               string                               `yaml:"os" doc:"operating system of the tart VM" tview:"widget=dropdown,options=macos|linux"`
	StateBackoff     ratecontrol.ExponentialBackoffConfig `yaml:"state_backoff" doc:"backoff when polling VM state"`
	RunBackoff       ratecontrol.ExponentialBackoffConfig `yaml:"run_backoff" doc:"backoff when waiting for VM to run"`
	ForceStopBackoff ratecontrol.ExponentialBackoffConfig `yaml:"force_stop_backoff" doc:"backoff when forcefully stopping a VM"`
	RunOptions       []string                             `yaml:"run_options,flow" doc:"additional options to pass to tart run" tview:"widget=csv"`
	TartBinary       string                               `yaml:"binary" doc:"path to the tart binary"`
}

type TartConfig struct {
	TartVMConfig         `yaml:",inline"`
	ociimage.Source      `yaml:",inline"`
	basepool.VMResources `yaml:",inline"`
	RunnerDir            string `yaml:"runner_dir" doc:"directory on the VM in which the runner is installed"`
	CosignBinary         string `yaml:"cosign_binary" doc:"path to cosign binary"`
}

type MockConfig struct {
	RunnerDir   string        `yaml:"runner_dir" doc:"mock guest runner directory"`
	StartupWait time.Duration `yaml:"startup_wait" doc:"simulated VM startup latency"`
}

type PoolConfig struct {
	basepool.Config `yaml:",inline"`
	Tart            *TartConfig `yaml:"tart_config" doc:"configure and select the tart VM backend for this pool"`
	Mock            *MockConfig `yaml:"mock_config" doc:"configure and select the mock VM backend for this pool"`
	Backend         any         `yaml:"-"`
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

type WebhookConfig struct {
	RelayURL    string               `yaml:"relay_url" doc:"the URL to which GitHub will send webhook events"`
	RateControl crawlcmd.RateControl `yaml:"rate_control" doc:"rate control settings for the webhook relay service"`
	Streaming   bool                 `yaml:"streaming" doc:"use the streaming webhook API instead of polling"`
}

type JWTIssuerConfig struct {
	jwtutil.JWTCookieSignerConfig `yaml:",inline"`
	SigningKey                    keys.KeySpec `yaml:"jwt_signing_key" doc:"jwt signing key spec"`
	Redirect                      string       `yaml:"redirect" doc:"URL to redirect to after setting the cookie"`
}

func (jc *JWTIssuerConfig) Validate() error {
	if err := jc.JWTCookieSignerConfig.Validate(); err != nil {
		return err
	}
	return jc.JWTCookieConfig.Validate()
}

type JWTValidatorConfig struct {
	jwtutil.JWTCookieValidatorConfig `yaml:",inline"`
	VerificationKeys                 []keys.KeySpec `yaml:"jwt_verification_keys" doc:"jwt verification key specs" tview:"itemlabel=ID"`
}

type WebUIConfig struct {
	Enabled         bool                `yaml:"enabled" doc:"enable the web UI and management API"`
	ListenAddress   string              `yaml:"listen_address" doc:"address for the web UI/API HTTP server"`
	Reload          webassets.Config    `yaml:"reload" doc:"serve the web UI assets from the local filesystem"`
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
	cookie := jwtutil.JWTCookieConfig{
		Name: "github-runner-orchestrator",
		ScopeAndDuration: cookies.ScopeAndDuration{
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
			LoggingConfig: cmdutil.LoggingConfig{
				Level:  2,
				File:   "-",
				Format: "text",
			},
			MaxSize:    10 * cmdyaml.MB,
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
				Type:          flags.Enum[KeychainType]{Value: KeychainAll},
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
				Config: basepool.Config{
					Size:                  2,
					CleanupTimeout:        time.Minute,
					StopTimeout:           30 * time.Second,
					StagingBehaviour:      basepool.StagingBehaviourRunning,
					DeleteAcquiredOnClose: true,
					CreateBackoff:         ratecontrol.ExponentialBackoffConfig{InitialDelay: 500 * time.Millisecond, Steps: 10},
				},
				Tart: &TartConfig{
					TartVMConfig: TartVMConfig{
						OS:         "macos",
						TartBinary: "/opt/homebrew/bin/tart",
					},
					Source: ociimage.Source{
						Image:    "mm-1:5001/macos-ci:latest",
						Insecure: true,
						Verify: &ociimage.Policy{
							Signers: []ociimage.Signer{{Key: "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----"}},
						},
					},
					VMResources:  basepool.VMResources{Cores: 4, MemGiB: 8},
					RunnerDir:    "/Users/admin/actions-runner",
					CosignBinary: "/opt/homebrew/bin/cosign",
				},
			},
			"linux": {
				Config: basepool.Config{
					Size:                  2,
					StagingBehaviour:      basepool.StagingBehaviourStopped,
					DeleteAcquiredOnClose: true,
				},
				Tart: &TartConfig{
					TartVMConfig: TartVMConfig{
						OS:         "linux",
						RunOptions: []string{"--no-graphics", "--no-audio", "--nested"},
						TartBinary: "/opt/homebrew/bin/tart",
					},
					Source: ociimage.Source{
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
			RateControl: crawlcmd.RateControl{
				ExponentialBackoff: crawlcmd.ExponentialBackoff{
					ExponentialBackoffConfig: ratecontrol.ExponentialBackoffConfig{
						InitialDelay: time.Second,
						Steps:        10,
					},
					StatusCodes: []int{429, 502, 503},
				},
			},
		},
		WebUI: WebUIConfig{
			Enabled:       true,
			ListenAddress: "127.0.0.1:8089",
			JWTVerification: &JWTValidatorConfig{
				JWTCookieValidatorConfig: jwtutil.JWTCookieValidatorConfig{
					JWTCookieConfig:    cookie,
					ValidationTimeSkew: time.Minute,
					JWTValidatorConfig: jwtutil.JWTValidatorConfig{
						Issuer:   "github-orchestrator",
						Audience: []string{"github-orchestrator"},
						Subject:  "github-orchestrator",
						Claims:   map[string]string{"access": "admin"},
					},
				},
				VerificationKeys: []keys.KeySpec{{User: "cnicolaou", ID: "github-orchestrator-jwt-key"}},
			},
		},
		JWTIssuer: &JWTIssuerConfig{
			JWTCookieSignerConfig: jwtutil.JWTCookieSignerConfig{
				JWTCookieConfig: cookie,
				JWTSignerConfig: jwtutil.JWTSignerConfig{
					Issuer:   "github-orchestrator",
					Audience: []string{"github-orchestrator"},
					Duration: 24 * time.Hour,
					Subject:  "github-orchestrator",
					Claims:   map[string]string{"access": "admin"},
				},
			},
			SigningKey: keys.KeySpec{User: "cnicolaou", ID: "github-orchestrator-jwt-key"},
		},
	}
}
