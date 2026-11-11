// Command serverroom manages your dedicated servers from a terminal with an API key.
//
//	serverroom login                 # paste an API key from your control panel (API & Keys)
//	serverroom servers list
//	serverroom servers get 246447
//	serverroom stock --location "New York" --max-price 50
//	serverroom reinstall 246447 --os "Ubuntu 24.04" --ssh-key laptop --yes
//
// The key's prefix says which website it was made on, so the CLI talks to that website's API with no brand setting.
// --json on any command prints JSON. Exit status: 0 done, 1 refused or failed, 2 bad usage.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/primcast/terraform-provider-serverroom/client"
)

// version, commit are set by the release build.
var (
	version = "dev"
	commit  = ""
)

var (
	jsonOut     bool
	profileName string
	apiURLFlag  string
)

type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

func usagef(format string, a ...interface{}) error { return usageError{fmt.Sprintf(format, a...)} }

func main() {
	root := &cobra.Command{
		Use:           "serverroom",
		Short:         "Manage your dedicated servers with an API key",
		Long:          "Manage your dedicated servers from a terminal.\n\nCreate an API key in your control panel under API & Keys, then run `serverroom login`.\nThe key decides which account and website the CLI talks to.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "print JSON")
	root.PersistentFlags().StringVar(&profileName, "profile", "", "saved login to use (default: SERVERROOM_PROFILE, else \"default\")")
	root.PersistentFlags().StringVar(&apiURLFlag, "api-url", "", "API base URL (default: SERVERROOM_API_URL, else the key's own website)")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err.Error()} })

	root.AddCommand(loginCmd(), logoutCmd(), whoamiCmd(), serversCmd(), powerCmd(), rebootCmd(), reinstallCmd(),
		rescueCmd(), sshKeysCmd(), stockCmd(), osCmd(), orderCmd(), rdnsCmd(), monitorsCmd(), webhooksCmd(),
		portAlertCmd(), versionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "serverroom: "+err.Error())
		var u usageError
		if errors.As(err, &u) || strings.HasPrefix(err.Error(), "unknown command") ||
			strings.HasPrefix(err.Error(), "accepts ") || strings.HasPrefix(err.Error(), "requires ") {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print the version", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if jsonOut {
				return printJSON(map[string]string{"version": version, "commit": commit})
			}
			fmt.Println("serverroom", version)
			return nil
		}}
}

// ------------------------------------------------------------------------------------------------- config

// Profile is one saved login.
type Profile struct {
	Key    string `json:"key"`
	APIURL string `json:"api_url,omitempty"`
}

// Config is the CLI's file: saved logins and named SSH public keys.
type Config struct {
	Profiles map[string]Profile `json:"profiles"`
	SSHKeys  map[string]string  `json:"ssh_keys"`
}

func configPath() (string, error) {
	if p := os.Getenv("SERVERROOM_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "serverroom", "config.json"), nil
}

func loadConfig() (*Config, error) {
	cfg := &Config{Profiles: map[string]Profile{}, SSHKeys: map[string]string{}}
	p, err := configPath()
	if err != nil {
		return cfg, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return cfg, fmt.Errorf("%s is not valid JSON: %w", p, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if cfg.SSHKeys == nil {
		cfg.SSHKeys = map[string]string{}
	}
	return cfg, nil
}

func saveConfig(cfg *Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func currentProfile() string {
	if profileName != "" {
		return profileName
	}
	if p := os.Getenv("SERVERROOM_PROFILE"); p != "" {
		return p
	}
	return "default"
}

// newClient builds the client: the key from SERVERROOM_API_KEY or the saved profile, the base URL from --api-url,
// SERVERROOM_API_URL, the profile, else the key's own website.
func newClient() (*client.Client, error) {
	key := os.Getenv("SERVERROOM_API_KEY")
	base := firstNonEmpty(apiURLFlag, os.Getenv("SERVERROOM_API_URL"))
	if key == "" {
		cfg, err := loadConfig()
		if err != nil {
			return nil, err
		}
		prof, ok := cfg.Profiles[currentProfile()]
		if !ok || prof.Key == "" {
			return nil, usagef("not logged in: run `serverroom login` (or set SERVERROOM_API_KEY)")
		}
		key = prof.Key
		base = firstNonEmpty(base, prof.APIURL)
	}
	if err := client.CheckKey(base, key); err != nil {
		return nil, usagef("give --api-url (or SERVERROOM_API_URL) https://<your control panel website>/api: this key's website is not known to this version")
	}
	return makeClient(base, key), nil
}

func makeClient(base, key string) *client.Client {
	c := client.New(base, key)
	client.Version = version
	c.Agent = "serverroom-cli/" + version
	return c
}

// publicClient is for the routes that need no key (stock, plans): logged in or not.
func publicClient() *client.Client {
	if c, err := newClient(); err == nil {
		return c
	}
	return makeClient(firstNonEmpty(apiURLFlag, os.Getenv("SERVERROOM_API_URL")), "")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ------------------------------------------------------------------------------------------------- output

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func table(header string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, header)
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	w.Flush()
}

func accountArg(args []string) (int, error) {
	if len(args) == 0 {
		return 0, usagef("missing service id (the number `serverroom servers list` shows)")
	}
	n, err := strconv.Atoi(strings.TrimPrefix(args[0], "#"))
	if err != nil || n <= 0 {
		return 0, usagef("service id must be a number, not %q", args[0])
	}
	return n, nil
}

func readFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "-" {
		raw, err := readAllStdin()
		return string(raw), err
	}
	raw, err := os.ReadFile(expandHome(path))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func statusWord(status int) string {
	switch status {
	case 21, 26, 28, 7, 8, 9, 10, 11, 12, 19, 22, 23, 25, 27, 102:
		return "active"
	case 4, 6, 300, 301, 302, 303:
		return "setting up"
	case 0, 1, 2, 3:
		return "awaiting payment"
	case 13, 14, 15, 16, 17, 18, 30, 31, 32, 44, 113:
		return "suspended"
	}
	return "closed"
}
