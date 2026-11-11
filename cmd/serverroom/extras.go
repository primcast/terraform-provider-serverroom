package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/primcast/terraform-provider-serverroom/client"
)

// ------------------------------------------------------------------------------------------------- stock

func stockCmd() *cobra.Command {
	var f client.StockFilter
	cmd := &cobra.Command{Use: "stock", Short: "Servers in stock now, cheapest first, with prices",
		Long: "Machines ready to deliver now. Prices are monthly in USD; a discounted machine shows its first-month\n" +
			"price and, after it, the price from the second month on. Filter values match loosely: \"Amsterdam\" finds\n" +
			"\"Amsterdam, EU\". `serverroom stock options location` lists what a filter takes.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			list, err := publicClient().Stock(f)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(list)
			}
			if len(list) == 0 {
				fmt.Println("nothing in stock matches")
				return nil
			}
			rows := [][]string{}
			for _, s := range list {
				price := fmt.Sprintf("$%.2f", s.Price)
				if s.FirstMonth && s.ListPrice > s.Price {
					price = fmt.Sprintf("$%.2f then $%.2f", s.Price, s.ListPrice)
				}
				hw := fmt.Sprintf("%s, %g GB, %s", s.CPU, s.RAM, s.Disk)
				if s.GPU != "" {
					hw += ", " + s.GPU
				}
				rows = append(rows, []string{s.Name, s.Type, hw, s.Location, s.Traffic, price})
			}
			table("SERVER\tPLAN\tHARDWARE\tLOCATION\tTRAFFIC\tPRICE/MO", rows)
			fmt.Println("\norder one: serverroom order <server>")
			return nil
		}}
	cmd.Flags().StringSliceVar(&f.Location, "location", nil, "city or region (repeatable)")
	cmd.Flags().StringSliceVar(&f.Type, "plan", nil, "plan, e.g. M700 (repeatable)")
	cmd.Flags().StringSliceVar(&f.CPU, "cpu", nil, "CPU (repeatable)")
	cmd.Flags().StringSliceVar(&f.GPU, "gpu", nil, "GPU (repeatable)")
	cmd.Flags().StringSliceVar(&f.OS, "os", nil, "installed OS (repeatable)")
	cmd.Flags().IntVar(&f.MinRAM, "min-ram", 0, "at least this many GB of memory")
	cmd.Flags().Float64Var(&f.MaxPrice, "max-price", 0, "at most this first-month price in USD")
	cmd.Flags().IntVar(&f.Limit, "limit", 20, "how many to show (max 40)")

	cmd.AddCommand(&cobra.Command{Use: "options <filter>", Short: "Values a stock filter takes: location, cpu, gpu, ram, type, os, traffic",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			values, err := publicClient().StockOptions(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(values)
			}
			for _, v := range values {
				fmt.Println(v)
			}
			return nil
		}})
	return cmd
}

func osCmd() *cobra.Command {
	return &cobra.Command{Use: "os <plan>", Short: "Operating systems a plan installs (names for reinstall --os)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			names, err := publicClient().PlanOS(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(names)
			}
			for _, n := range names {
				fmt.Println(n)
			}
			return nil
		}}
}

// ------------------------------------------------------------------------------------------------- order

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}

func orderCmd() *cobra.Command {
	var open bool
	cmd := &cobra.Command{Use: "order <server>", Short: "Order a server in stock (opens its checkout)",
		Long: "Find an in-stock server (e.g. s924 from `serverroom stock`) and open its order page, prefilled with\n" +
			"that machine. You review the build and pay there: an API key can never spend money.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := strings.ToLower(strings.TrimSpace(args[0]))
			if _, err := strconv.Atoi(name); err == nil {
				name = "s" + name
			}
			c := publicClient()
			list, err := c.Stock(client.StockFilter{MaxPrice: 1e9, Limit: 40})
			if err != nil {
				return err
			}
			var found *client.StockServer
			for i := range list {
				if strings.EqualFold(list[i].Name, name) {
					found = &list[i]
				}
			}
			if found == nil {
				// The list is capped; ask by the plan family too before giving up.
				plans, _ := c.StockOptions("type")
				for _, plan := range plans {
					more, err := c.Stock(client.StockFilter{Type: []string{plan}, Limit: 40})
					if err != nil {
						continue
					}
					for i := range more {
						if strings.EqualFold(more[i].Name, name) {
							found = &more[i]
						}
					}
					if found != nil {
						break
					}
				}
			}
			if found == nil {
				return fmt.Errorf("%s is not in stock (sold, or not ready yet); see serverroom stock", args[0])
			}
			if jsonOut {
				return printJSON(found)
			}
			fmt.Printf("%s: %s, %g GB, %s in %s, $%.2f/mo\n", found.Name, found.CPU, found.RAM, found.Disk, found.Location, found.Price)
			fmt.Println("order page:", found.OrderURL)
			if open && found.OrderURL != "" {
				return openBrowser(found.OrderURL)
			}
			return nil
		}}
	cmd.Flags().BoolVar(&open, "open", true, "open the order page in your browser")
	return cmd
}

// ------------------------------------------------------------------------------------------------- rdns

func rdnsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "rdns", Short: "Reverse DNS of your addresses"}
	cmd.AddCommand(
		&cobra.Command{Use: "get <ip>", Short: "Read a PTR name", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				name, err := c.Reverse(args[0])
				if err != nil {
					return err
				}
				if jsonOut {
					return printJSON(map[string]string{"ip": args[0], "reverse": name})
				}
				fmt.Println(name)
				return nil
			}},
		&cobra.Command{Use: "set <ip> <name>", Short: "Set a PTR name", Args: cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				me, err := c.Whoami()
				if err != nil {
					return err
				}
				if err := c.SetReverse(me.Dbid, args[0], strings.TrimSuffix(args[1], ".")); err != nil {
					return err
				}
				fmt.Println("reverse DNS of", args[0], "set to", args[1])
				return nil
			}},
	)
	return cmd
}

// ------------------------------------------------------------------------------------------------- monitors

func monitorsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "monitors", Aliases: []string{"monitor"}, Short: "Your website / TCP / ping monitors"}
	list := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "Your monitors", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			all, err := c.Monitors()
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(all)
			}
			rows := [][]string{}
			for _, m := range all {
				target := m.Target
				if m.Type == "tcp" {
					target = fmt.Sprintf("%s:%d", m.Target, m.Port)
				}
				rows = append(rows, []string{m.ID, m.Name, m.Type, target, fmt.Sprintf("%ds", m.Interval), m.State.Status})
			}
			table("ID\tNAME\tTYPE\tTARGET\tEVERY\tSTATE", rows)
			return nil
		}}
	var m client.Monitor
	add := &cobra.Command{Use: "add", Short: "Add a monitor", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if m.AccountID == 0 || m.Name == "" || m.Target == "" {
				return usagef("monitors add needs --id, --name and --target")
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			made, err := c.CreateMonitor(m)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(made)
			}
			fmt.Println("monitor", made.ID, "added")
			return nil
		}}
	add.Flags().IntVar(&m.AccountID, "id", 0, "service the monitor belongs to")
	add.Flags().StringVar(&m.Name, "name", "", "name")
	add.Flags().StringVar(&m.Type, "type", "http", "http, tcp or ping")
	add.Flags().StringVar(&m.Target, "target", "", "URL (http) or host")
	add.Flags().IntVar(&m.Port, "port", 0, "port (tcp)")
	add.Flags().IntVar(&m.Interval, "interval", 0, "seconds between checks (default: your support tier's shortest)")
	add.Flags().IntVar(&m.Confirm, "confirm", 0, "failed checks in a row before an alert")
	add.Flags().StringVar(&m.Keyword, "keyword", "", "word the page must contain (http)")
	test := &cobra.Command{Use: "test <monitor-id>", Short: "Run a monitor once now", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			r, err := c.TestMonitor(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(r)
			}
			if r.OK {
				fmt.Printf("up, %d ms\n", r.Ms)
				return nil
			}
			return fmt.Errorf("down: %s", r.Error)
		}}
	rm := &cobra.Command{Use: "rm <monitor-id>", Aliases: []string{"delete"}, Short: "Delete a monitor", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			if err := c.DeleteMonitor(args[0]); err != nil {
				return err
			}
			fmt.Println("monitor", args[0], "deleted")
			return nil
		}}
	cmd.AddCommand(list, add, test, rm)
	return cmd
}

// ------------------------------------------------------------------------------------------------- webhooks

func webhooksCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "webhooks", Aliases: []string{"webhook"}, Short: "Endpoints that receive your account's events"}
	list := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "Your endpoints", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			all, err := c.Webhooks()
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(all)
			}
			rows := [][]string{}
			for _, w := range all {
				on := "on"
				if w.Active != nil && !*w.Active {
					on = "off"
				}
				rows = append(rows, []string{w.ID, w.URL, strings.Join(w.Events, ","), on})
			}
			table("ID\tURL\tEVENTS\tSTATE", rows)
			return nil
		}}
	var w client.Webhook
	add := &cobra.Command{Use: "add", Short: "Add an endpoint", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if w.URL == "" {
				return usagef("webhooks add needs --url")
			}
			if len(w.Events) == 0 {
				w.Events = []string{"*"}
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			made, err := c.CreateWebhook(w)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(made)
			}
			fmt.Println("webhook", made.ID, "added; its signing secret (shown once):", made.Secret)
			return nil
		}}
	add.Flags().StringVar(&w.URL, "url", "", "https:// endpoint")
	add.Flags().StringSliceVar(&w.Events, "events", nil, "event types (comma-separated), default all")
	add.Flags().StringVar(&w.Description, "description", "", "description")
	rm := &cobra.Command{Use: "rm <webhook-id>", Aliases: []string{"delete"}, Short: "Delete an endpoint", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			if err := c.DeleteWebhook(args[0]); err != nil {
				return err
			}
			fmt.Println("webhook", args[0], "deleted")
			return nil
		}}
	cmd.AddCommand(list, add, rm)
	return cmd
}

// ------------------------------------------------------------------------------------------------- port alert

func portAlertCmd() *cobra.Command {
	var enable, disable bool
	var threshold, sustain int
	var direction string
	cmd := &cobra.Command{Use: "port-alert <id>", Short: "Read or set the alert for a busy network port", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := accountArg(args)
			if err != nil {
				return err
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			r, err := c.PortRule(id)
			if err != nil {
				return err
			}
			changing := enable || disable || cmd.Flags().Changed("threshold") || cmd.Flags().Changed("sustain") || cmd.Flags().Changed("direction")
			if changing {
				if enable {
					r.Enabled = true
				}
				if disable {
					r.Enabled = false
				}
				if cmd.Flags().Changed("threshold") {
					r.Threshold = threshold
				}
				if cmd.Flags().Changed("sustain") {
					r.Sustain = sustain
				}
				if cmd.Flags().Changed("direction") {
					r.Direction = direction
				}
				r.PortSpeedMbps = 0
				if r, err = c.SetPortRule(id, *r); err != nil {
					return err
				}
			}
			if jsonOut {
				return printJSON(r)
			}
			state := "off"
			if r.Enabled {
				state = "on"
			}
			fmt.Printf("%s: above %d%% of %d Mbps for %d min (%s)\n", state, r.Threshold, r.PortSpeedMbps, r.Sustain, r.Direction)
			return nil
		}}
	cmd.Flags().BoolVar(&enable, "enable", false, "turn it on")
	cmd.Flags().BoolVar(&disable, "disable", false, "turn it off")
	cmd.Flags().IntVar(&threshold, "threshold", 90, "percent of the port speed, 50-100")
	cmd.Flags().IntVar(&sustain, "sustain", 15, "minutes, 10-240")
	cmd.Flags().StringVar(&direction, "direction", "either", "either, inbound or outbound")
	return cmd
}
