// Command bacnetf (bacnetfinder) is a cross-platform BACnet/IP command-line
// client for discovering devices, exploring their objects and properties, and
// reading and writing property values — a scriptable counterpart to graphical
// tools such as YABE, aimed at commissioning workflows.
//
// It is a thin front-end over the github.com/worldiety/bacnet/client package,
// which provides the reusable, ergonomic BACnet client API.
//
// Subcommands:
//
//	discover   broadcast Who-Is and list responding devices
//	objects    list all objects on a device (reads its object-list)
//	props      read several properties of one object
//	read       read a single property of an object
//	write      write a single property (guarded; mutates a live device)
//
// Run "bacnetf <command> -h" for command-specific flags.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/worldiety/bacnet/client"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "discover", "disco":
		err = cmdDiscover(args)
	case "objects", "objs", "ls":
		err = cmdObjects(args)
	case "props", "properties":
		err = cmdProps(args)
	case "read", "rp":
		err = cmdRead(args)
	case "write", "wp":
		err = cmdWrite(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	if err != nil {
		// flag.ErrHelp is returned when the user asked for -h on a subcommand.
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// cliOptions holds the flags shared by every subcommand. They are translated
// into a client.Config by config().
type cliOptions struct {
	iface         string
	bcast         string
	timeout       time.Duration
	retries       int
	delay         time.Duration
	resolveWindow time.Duration
	verbose       bool
}

// registerCommonFlags registers the flags shared by every subcommand.
func registerCommonFlags(fs *flag.FlagSet, opts *cliOptions) {
	fs.StringVar(&opts.iface, "iface", "", "local IPv4 to bind (default: all interfaces; recommended for discovery so broadcast I-Am replies are received)")
	fs.StringVar(&opts.bcast, "bcast", "", "broadcast IPv4 for Who-Is (default: auto-detect every interface)")
	fs.DurationVar(&opts.timeout, "timeout", 10*time.Second, "per-request timeout (raise for slow MS/TP lines)")
	fs.IntVar(&opts.retries, "retries", 3, "APDU retransmission attempts")
	fs.DurationVar(&opts.delay, "delay", 0, "delay inserted between sequential requests (gentle pacing for MS/TP)")
	fs.DurationVar(&opts.resolveWindow, "resolve-window", 2*time.Second, "how long to wait for I-Am when resolving a device ID to an address")
	fs.BoolVar(&opts.verbose, "v", false, "verbose: enable debug logging from the BACnet stack")
}

// config builds a client.Config from the CLI options.
func (o cliOptions) config() (client.Config, error) {
	cfg := client.Config{
		Timeout:       o.timeout,
		Retries:       o.retries,
		Pacing:        o.delay,
		ResolveWindow: o.resolveWindow,
	}
	if o.iface != "" {
		a, err := netip.ParseAddr(o.iface)
		if err != nil || !a.Is4() {
			return client.Config{}, fmt.Errorf("invalid --iface %q: must be an IPv4 address", o.iface)
		}
		cfg.Interface = a
	}
	if o.bcast != "" {
		a, err := netip.ParseAddr(o.bcast)
		if err != nil || !a.Is4() {
			return client.Config{}, fmt.Errorf("invalid --bcast %q: must be an IPv4 address", o.bcast)
		}
		cfg.Broadcast = a
	}
	if o.verbose {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return cfg, nil
}

// newClient builds a client.Config from opts and opens a client.
func newClient(opts cliOptions) (*client.Client, error) {
	cfg, err := opts.config()
	if err != nil {
		return nil, err
	}
	return client.New(cfg)
}

// parseArgs parses a flag set while allowing flags and positional arguments to
// be interspersed in any order (unlike the stdlib flag package, which stops at
// the first positional). This lets a flag be placed after positional operands,
// e.g. "bacnetf write 5123 analog-value:1 present-value real:21.5 --priority 8".
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	return positional, nil
}

// confirm prompts the operator and returns true only for an explicit yes.
func confirm(prompt string) (bool, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		// Treat EOF / no tty as "no".
		return false, nil
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `bacnetf (bacnetfinder) - cross-platform BACnet/IP CLI

Usage:
  bacnetf <command> [arguments] [flags]

Commands:
  discover                                   broadcast Who-Is and list devices
  objects  <device>                          list all objects on a device
  props    <device> <object> [property...]   read properties of an object
  read     <device> <object> <property>      read a single property
  write    <device> <object> <property> <type:value>
                                             write a single property (guarded)

Addressing:
  <device>    device ID or IP     e.g. 5123, device:5123, #5123, 10.6.6.123[:47808]
                                  (a device ID is resolved to its address via Who-Is)
  <object>    <type>:<instance>   e.g. analog-value:1, device:5123, 2:1
  <property>  <name>|<number>     e.g. present-value, object-name, 85

Examples:
  bacnetf discover
  bacnetf objects 5123
  bacnetf props   5123 device:5123
  bacnetf read    5123 analog-value:1 present-value
  bacnetf write   5123 analog-value:1 present-value real:21.5 --priority 8

Run "bacnetf <command> -h" for command-specific flags.
`)
}
