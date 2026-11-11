package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/primcast/terraform-provider-serverroom/client"
)

func readAllStdin() ([]byte, error) { return io.ReadAll(os.Stdin) }

// ------------------------------------------------------------------------------------------------- login

func loginCmd() *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save an API key (checked before it is saved)",
		Long: "Save an API key for this computer. Create one in your control panel under API & Keys.\n\n" +
			"The key is read from --key, else from standard input (paste it, or pipe it in). Use --profile to keep\n" +
			"keys for several accounts side by side. The key is stored in your user config folder, readable only by you.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if key == "" {
				if fi, _ := os.Stdin.Stat(); fi != nil && fi.Mode()&os.ModeCharDevice != 0 {
					fmt.Fprint(os.Stderr, "API key: ")
				}
				line, err := bufio.NewReader(os.Stdin).ReadString('\n')
				if err != nil && line == "" {
					return usagef("no key given")
				}
				key = strings.TrimSpace(line)
			}
			if key == "" {
				return usagef("no key given")
			}
			base := firstNonEmpty(apiURLFlag, os.Getenv("SERVERROOM_API_URL"))
			if err := client.CheckKey(base, key); err != nil {
				return usagef("give --api-url https://<your control panel website>/api with this key: its website is not known to this version")
			}
			c := makeClient(base, key)
			me, err := c.Whoami()
			if err != nil {
				return fmt.Errorf("the key was not accepted: %w", err)
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			cfg.Profiles[currentProfile()] = Profile{Key: key, APIURL: base}
			if err := saveConfig(cfg); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(me)
			}
			fmt.Printf("Logged in as customer %d on %s (profile %q).\n", me.UID, firstNonEmpty(me.Brand, c.Brand), currentProfile())
			return nil
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "the API key (safer: paste it when asked, so it stays out of your shell history)")
	return cmd
}

func logoutCmd() *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Forget the saved API key of this profile", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			delete(cfg.Profiles, currentProfile())
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("Profile %q forgotten. The key itself still works until you revoke it in the control panel.\n", currentProfile())
			return nil
		}}
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{Use: "whoami", Short: "Who the key belongs to and what it may do", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			me, err := c.Whoami()
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(me)
			}
			fmt.Printf("customer %d on %s, %d service(s)\n", me.UID, firstNonEmpty(me.Brand, c.Brand), len(me.Accounts))
			if me.Key.ID != "" {
				fmt.Printf("key %s (%s): %s\n", me.Key.ID, me.Key.Name, strings.Join(me.Key.Scopes, ", "))
			}
			return nil
		}}
}

// ------------------------------------------------------------------------------------------------- servers

func serversCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "servers", Aliases: []string{"server"}, Short: "List and inspect your services"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "Every service on your account", Args: cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error { return listServers() }},
		&cobra.Command{Use: "get <id>", Short: "One service: features and IP addresses", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error { return getServer(args) }},
		&cobra.Command{Use: "rename <id> <name>", Short: "Rename a service (a server's name is its host name)", Args: cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				id, err := accountArg(args)
				if err != nil {
					return err
				}
				c, err := newClient()
				if err != nil {
					return err
				}
				if err := c.RenameService(id, args[1]); err != nil {
					return err
				}
				fmt.Println("renamed", id, "to", args[1])
				return nil
			}},
		progressCmd(),
	)
	return cmd
}

func listServers() error {
	c, err := newClient()
	if err != nil {
		return err
	}
	me, err := c.Whoami()
	if err != nil {
		return err
	}
	list, err := c.Services(me.UID)
	if err != nil {
		return err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].AccountID < list[j].AccountID })
	if jsonOut {
		return printJSON(list)
	}
	rows := [][]string{}
	for _, s := range list {
		rows = append(rows, []string{strconv.Itoa(s.AccountID), s.AccountType, s.AccountName, statusWord(s.Status)})
	}
	table("ID\tPLAN\tNAME\tSTATUS", rows)
	return nil
}

// serverID is the machine behind a service, from its features.
func serverID(c *client.Client, accountID int) (int, map[string]string, error) {
	features, err := c.Features(accountID)
	if err != nil {
		return 0, nil, err
	}
	fm := client.FeatureMap(features)
	sid, convErr := strconv.Atoi(fm["ServerID"])
	if convErr != nil || sid <= 0 {
		return 0, fm, fmt.Errorf("service %d has no server yet", accountID)
	}
	return sid, fm, nil
}

func getServer(args []string) error {
	id, err := accountArg(args)
	if err != nil {
		return err
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	sid, fm, err := serverID(c, id)
	if fm == nil {
		return err
	}
	var ips []client.IP
	if sid > 0 {
		ips, _ = c.IPs(id, sid)
	}
	if jsonOut {
		return printJSON(map[string]interface{}{"id": id, "server_id": sid, "features": fm, "ips": ips})
	}
	rows := [][]string{}
	for _, k := range sortedKeys(fm) {
		rows = append(rows, []string{k, fm[k]})
	}
	table("FEATURE\tVALUE", rows)
	if len(ips) > 0 {
		fmt.Println()
		ipRows := [][]string{}
		for _, ip := range ips {
			ipRows = append(ipRows, []string{ip.IP, ip.Reverse})
		}
		table("ADDRESS\tREVERSE DNS", ipRows)
	}
	return nil
}

func progressCmd() *cobra.Command {
	var watch bool
	cmd := &cobra.Command{Use: "progress [id]", Short: "Where installs and reinstalls stand", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id := 0
			if len(args) == 1 {
				var err error
				if id, err = accountArg(args); err != nil {
					return err
				}
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			for {
				list, err := c.SetupProgress(id)
				if err != nil {
					return err
				}
				if jsonOut {
					if err := printJSON(list); err != nil || !watch {
						return err
					}
				} else {
					if len(list) == 0 {
						fmt.Println("nothing installing")
					}
					rows := [][]string{}
					for _, p := range list {
						rows = append(rows, []string{strconv.Itoa(p.AccountID), p.OS, p.Stage, p.State, fmt.Sprintf("%d%%", p.Percent)})
					}
					if len(rows) > 0 {
						table("ID\tOS\tSTAGE\tSTATE\tDONE", rows)
					}
				}
				done := true
				for _, p := range list {
					if p.State == "running" {
						done = false
					}
				}
				if !watch || done {
					return nil
				}
				time.Sleep(20 * time.Second)
				if !jsonOut {
					fmt.Println()
				}
			}
		}}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "keep checking every 20 s until nothing is running")
	return cmd
}

// ------------------------------------------------------------------------------------------------- power

func doReboot(id int, yes bool) error {
	if !yes {
		return usagef("this restarts server %d now; add --yes to go ahead", id)
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	if err := c.Reboot(id); err != nil {
		return err
	}
	fmt.Println("reboot requested for", id)
	return nil
}

func powerCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "power", Short: "Power actions on a server"}
	reboot := &cobra.Command{Use: "reboot <id>", Aliases: []string{"cycle", "restart"}, Short: "Restart a server now",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			return doReboot(id, yes)
		}}
	reboot.Flags().BoolVar(&yes, "yes", false, "confirm")
	cmd.AddCommand(reboot)
	return cmd
}

func rebootCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "reboot <id>", Short: "Restart a server now (same as power reboot)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			return doReboot(id, yes)
		}}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm")
	return cmd
}

// ------------------------------------------------------------------------------------------------- reinstall

// sshKeysFrom joins saved key names and key files into one authorized_keys text.
func sshKeysFrom(names, files []string) (string, error) {
	var lines []string
	if len(names) > 0 {
		cfg, err := loadConfig()
		if err != nil {
			return "", err
		}
		for _, n := range names {
			k, ok := cfg.SSHKeys[n]
			if !ok {
				return "", usagef("no saved SSH key %q (serverroom ssh-keys list)", n)
			}
			lines = append(lines, k)
		}
	}
	for _, f := range files {
		text, err := readFile(f)
		if err != nil {
			return "", err
		}
		for _, l := range strings.Split(text, "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				lines = append(lines, l)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func reinstallCmd() *cobra.Command {
	var osName, dataFile string
	var keyNames, keyFiles []string
	var yes, wait bool
	cmd := &cobra.Command{Use: "reinstall <id>", Short: "ERASE a server and install an operating system",
		Long: "Reinstall the operating system. EVERYTHING ON THE SERVER IS ERASED.\n\n" +
			"--os takes the name exactly as the plan sells it (`serverroom os <plan>` lists them).\n" +
			"SSH keys and user data apply to Linux installs.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			if osName == "" {
				return usagef("give --os (serverroom os <plan> lists the names)")
			}
			if !yes {
				return usagef("reinstalling ERASES server %d; add --yes to go ahead", id)
			}
			keys, err := sshKeysFrom(keyNames, keyFiles)
			if err != nil {
				return err
			}
			data, err := readFile(dataFile)
			if err != nil {
				return err
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			if err := c.Reinstall(id, osName, keys, data); err != nil {
				return err
			}
			fmt.Println("reinstall of", osName, "started on", id, "-- it usually takes 10 to 30 minutes")
			if !wait {
				return nil
			}
			for {
				time.Sleep(30 * time.Second)
				list, err := c.SetupProgress(id)
				if err != nil {
					return err
				}
				if len(list) == 0 {
					continue
				}
				p := list[0]
				fmt.Printf("%s  %s %d%%\n", time.Now().Format("15:04:05"), p.Stage, p.Percent)
				switch p.State {
				case "ready":
					fmt.Println("done")
					return nil
				case "failed":
					return fmt.Errorf("the install failed at %s; our team has been told", p.Stage)
				}
			}
		}}
	cmd.Flags().StringVar(&osName, "os", "", "operating system, exactly as sold for the plan")
	cmd.Flags().StringSliceVar(&keyNames, "ssh-key", nil, "saved SSH key name (repeatable; see ssh-keys)")
	cmd.Flags().StringSliceVar(&keyFiles, "ssh-key-file", nil, "file of OpenSSH public keys (repeatable)")
	cmd.Flags().StringVar(&dataFile, "user-data", "", "file with #cloud-config or a #! script, run once after the install (- for stdin)")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm: everything on the server is erased")
	cmd.Flags().BoolVar(&wait, "wait", false, "follow the install until it is done")
	return cmd
}

// ------------------------------------------------------------------------------------------------- rescue

func rescueCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "rescue", Short: "Boot a server into the rescue system to repair it",
		Long: "Rescue mode powers the server off and boots it into a live Linux system, reachable over SSH as root\n" +
			"on the server's own IP, with its disks untouched. The key needs the Rescue mode permission."}
	var keyNames, keyFiles []string
	var yes bool
	start := &cobra.Command{Use: "start <id>", Short: "Start rescue mode (the server goes offline)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			if !yes {
				return usagef("rescue mode takes server %d offline until you end it; add --yes", id)
			}
			keys, err := sshKeysFrom(keyNames, keyFiles)
			if err != nil {
				return err
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			sid, _, err := serverID(c, id)
			if err != nil {
				return err
			}
			s, err := c.StartRescue(id, sid, keys)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(s)
			}
			fmt.Println("rescue mode starting on", id, "-- usually ready in 3 to 6 minutes (serverroom rescue status", id, ")")
			if s != nil && s.Password != "" {
				fmt.Printf("then: ssh %s@%s   password: %s\n", firstNonEmpty(s.Username, "root"), s.Address, s.Password)
			}
			return nil
		}}
	start.Flags().StringSliceVar(&keyNames, "ssh-key", nil, "saved SSH key name (repeatable)")
	start.Flags().StringSliceVar(&keyFiles, "ssh-key-file", nil, "file of OpenSSH public keys (repeatable)")
	start.Flags().BoolVar(&yes, "yes", false, "confirm")

	status := &cobra.Command{Use: "status <id>", Short: "Whether rescue mode runs or may start", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			sid, _, err := serverID(c, id)
			if err != nil {
				return err
			}
			st, err := c.Rescue(id, sid)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(st)
			}
			switch {
			case st.Session != nil && st.Session.Active:
				fmt.Printf("rescue mode: %s\n", st.Session.State)
				if st.Session.Address != "" {
					fmt.Printf("ssh %s@%s", firstNonEmpty(st.Session.Username, "root"), st.Session.Address)
					if st.Session.Password != "" {
						fmt.Printf("   password: %s", st.Session.Password)
					}
					fmt.Println()
				}
			case st.Available:
				fmt.Println("not running; can start (serverroom rescue start", id, "--yes)")
			default:
				fmt.Println("not available:", firstNonEmpty(st.Reason, "unknown reason"))
			}
			return nil
		}}

	var endYes bool
	end := &cobra.Command{Use: "end <id>", Aliases: []string{"stop"}, Short: "End rescue mode; the server boots from its disk", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			if !endYes {
				return usagef("this reboots server %d out of rescue mode; add --yes", id)
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			sid, _, err := serverID(c, id)
			if err != nil {
				return err
			}
			if err := c.EndRescue(id, sid); err != nil {
				return err
			}
			fmt.Println("rescue mode ending on", id, "; the server boots from its own disk")
			return nil
		}}
	end.Flags().BoolVar(&endYes, "yes", false, "confirm")
	cmd.AddCommand(start, status, end)
	return cmd
}

// ------------------------------------------------------------------------------------------------- ssh keys

func sshKeysCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ssh-keys", Aliases: []string{"ssh-key"}, Short: "Named SSH public keys for reinstall and rescue",
		Long: "Save public keys under a name, then use --ssh-key NAME with reinstall and rescue.\n" +
			"They are kept in this computer's CLI config; the server gets them at install time."}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "Saved keys", Args: cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				if jsonOut {
					return printJSON(cfg.SSHKeys)
				}
				rows := [][]string{}
				for _, n := range sortedKeys(cfg.SSHKeys) {
					f := strings.Fields(cfg.SSHKeys[n])
					kind, comment := "", ""
					if len(f) > 0 {
						kind = f[0]
					}
					if len(f) > 2 {
						comment = strings.Join(f[2:], " ")
					}
					rows = append(rows, []string{n, kind, comment})
				}
				table("NAME\tTYPE\tCOMMENT", rows)
				return nil
			}},
		&cobra.Command{Use: "add <name> <public-key-file>", Short: "Save a public key (e.g. ~/.ssh/id_ed25519.pub; - for stdin)", Args: cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				text, err := readFile(args[1])
				if err != nil {
					return err
				}
				text = strings.TrimSpace(text)
				if strings.Contains(text, "PRIVATE KEY") {
					return usagef("%s is a PRIVATE key; give the .pub file", args[1])
				}
				if !strings.HasPrefix(text, "ssh-") && !strings.HasPrefix(text, "ecdsa-") && !strings.HasPrefix(text, "sk-") {
					return usagef("%s does not look like an OpenSSH public key", args[1])
				}
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				cfg.SSHKeys[args[0]] = text
				if err := saveConfig(cfg); err != nil {
					return err
				}
				fmt.Println("saved SSH key", args[0])
				return nil
			}},
		&cobra.Command{Use: "rm <name>", Aliases: []string{"delete"}, Short: "Forget a saved key", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				if _, ok := cfg.SSHKeys[args[0]]; !ok {
					return usagef("no saved SSH key %q", args[0])
				}
				delete(cfg.SSHKeys, args[0])
				if err := saveConfig(cfg); err != nil {
					return err
				}
				fmt.Println("forgot SSH key", args[0])
				return nil
			}},
	)
	return cmd
}
