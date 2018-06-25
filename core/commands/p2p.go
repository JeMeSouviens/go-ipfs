package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	cmds "github.com/ipfs/go-ipfs/commands"
	core "github.com/ipfs/go-ipfs/core"
	p2p "github.com/ipfs/go-ipfs/p2p"

	ma "gx/ipfs/QmYmsdtJ3HsodkePE3eU3TsCaP2YvPZJ4LoXnNkDE5Tpt7/go-multiaddr"
	"gx/ipfs/QmZNkThpqfVXs9GNbexPrfBbXSLNYeKrE7jwFM2oqHbyqN/go-libp2p-protocol"
	pstore "gx/ipfs/QmZR2XWVVBCtbgBWnQhWk2xcQfaR3W8faQPriAiaaj7rsr/go-libp2p-peerstore"
	"gx/ipfs/QmdE4gMduCKCGAcczM2F5ioYDfdeKuPix138wrES1YSr7f/go-ipfs-cmdkit"
	"gx/ipfs/Qme4QgoVPyQqxVc4G1c2L2wc9TDa6o294rtspGMnBNRujm/go-ipfs-addr"
)

// P2PProtoPrefix is the default required prefix for protocol names
const P2PProtoPrefix = "/x/"

// P2PListenerInfoOutput is output type of ls command
type P2PListenerInfoOutput struct {
	Protocol      string
	ListenAddress string
	TargetAddress string
}

// P2PStreamInfoOutput is output type of streams command
type P2PStreamInfoOutput struct {
	HandlerID     string
	Protocol      string
	OriginAddress string
	TargetAddress string
}

// P2PLsOutput is output type of ls command
type P2PLsOutput struct {
	Listeners []P2PListenerInfoOutput
}

// P2PStreamsOutput is output type of streams command
type P2PStreamsOutput struct {
	Streams []P2PStreamInfoOutput
}

// P2PCmd is the 'ipfs p2p' command
var P2PCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Libp2p stream mounting.",
		ShortDescription: `
Create and use tunnels to remote peers over libp2p

Note: this command is experimental and subject to change as usecases and APIs
are refined`,
	},

	Subcommands: map[string]*cmds.Command{
		"stream": p2pStreamCmd,

		"forward": p2pForwardCmd,
		"listen":  p2pListenCmd,
		"close":   p2pCloseCmd,
		"ls":      p2pLsCmd,
	},
}

var p2pForwardCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Forward connections to libp2p service",
		ShortDescription: `
Forward connections made to <listen-address> to <target-address>.

<protocol> specifies the libp2p protocol name to use for libp2p
connections and/or handlers. It must be prefixed with '` + P2PProtoPrefix + `'.

Example:
  ipfs p2p forward ` + P2PProtoPrefix + `myproto /ip4/127.0.0.1/tcp/4567 /ipfs/QmPeer
    - Forward connections to 127.0.0.1:4567 to '` + P2PProtoPrefix + `myproto' service on /ipfs/QmPeer

`,
	},
	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("protocol", true, false, "Protocol name."),
		cmdkit.StringArg("listen-address", true, false, "Listening endpoint."),
		cmdkit.StringArg("target-address", true, false, "Target endpoint."),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("allow-custom-protocol", "Don't require /x/ prefix"),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		protoOpt := req.Arguments()[0]
		listenOpt := req.Arguments()[1]
		targetOpt := req.Arguments()[2]

		proto := protocol.ID(protoOpt)

		listen, err := ma.NewMultiaddr(listenOpt)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		target, err := ipfsaddr.ParseString(targetOpt)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		allowCustom, _, err := req.Option("allow-custom-protocol").Bool()
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		if !allowCustom && !strings.HasPrefix(string(proto), P2PProtoPrefix) {
			res.SetError(errors.New("protocol name must be within '"+P2PProtoPrefix+"' namespace"), cmdkit.ErrNormal)
			return
		}

		if err := forwardLocal(n.Context(), n.P2P, n.Peerstore, proto, listen, target); err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}
		res.SetOutput(nil)
	},
}

var p2pListenCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Create libp2p service",
		ShortDescription: `
Create libp2p service and forward connections made to <target-address>.

<protocol> specifies the libp2p handler name. It must be prefixed with '` + P2PProtoPrefix + `'.

Example:
  ipfs p2p listen ` + P2PProtoPrefix + `myproto /ip4/127.0.0.1/tcp/1234
    - Forward connections to 'myproto' libp2p service to 127.0.0.1:1234

`,
	},
	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("protocol", true, false, "Protocol name."),
		cmdkit.StringArg("target-address", true, false, "Target endpoint."),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("allow-custom-protocol", "Don't require /x/ prefix"),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		protoOpt := req.Arguments()[0]
		targetOpt := req.Arguments()[1]

		proto := protocol.ID(protoOpt)

		target, err := ma.NewMultiaddr(targetOpt)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		allowCustom, _, err := req.Option("allow-custom-protocol").Bool()
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		if !allowCustom && !strings.HasPrefix(string(proto), P2PProtoPrefix) {
			res.SetError(errors.New("protocol name must be within '"+P2PProtoPrefix+"' namespace"), cmdkit.ErrNormal)
			return
		}

		if err := forwardRemote(n.Context(), n.P2P, proto, target); err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		res.SetOutput(nil)
	},
}

// forwardRemote forwards libp2p service connections to a manet address
func forwardRemote(ctx context.Context, p *p2p.P2P, proto protocol.ID, target ma.Multiaddr) error {
	// TODO: return some info
	_, err := p.ForwardRemote(ctx, proto, target)
	return err
}

// forwardLocal forwards local connections to a libp2p service
func forwardLocal(ctx context.Context, p *p2p.P2P, ps pstore.Peerstore, proto protocol.ID, bindAddr ma.Multiaddr, addr ipfsaddr.IPFSAddr) error {
	if addr != nil {
		ps.AddAddr(addr.ID(), addr.Multiaddr(), pstore.TempAddrTTL)
	}

	// TODO: return some info
	_, err := p.ForwardLocal(ctx, addr.ID(), proto, bindAddr)
	return err
}

var p2pLsCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "List active p2p listeners.",
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("headers", "v", "Print table headers (Protocol, Listen, Target)."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		output := &P2PLsOutput{}

		for _, listener := range n.P2P.Listeners.Listeners {
			output.Listeners = append(output.Listeners, P2PListenerInfoOutput{
				Protocol:      string(listener.Protocol()),
				ListenAddress: listener.ListenAddress().String(),
				TargetAddress: listener.TargetAddress().String(),
			})
		}

		res.SetOutput(output)
	},
	Type: P2PLsOutput{},
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) (io.Reader, error) {
			v, err := unwrapOutput(res.Output())
			if err != nil {
				return nil, err
			}

			headers, _, _ := res.Request().Option("headers").Bool()
			list := v.(*P2PLsOutput)
			buf := new(bytes.Buffer)
			w := tabwriter.NewWriter(buf, 1, 2, 1, ' ', 0)
			for _, listener := range list.Listeners {
				if headers {
					fmt.Fprintln(w, "Protocol\tListen Address\tTarget Address")
				}

				fmt.Fprintf(w, "%s\t%s\t%s\n", listener.Protocol, listener.ListenAddress, listener.TargetAddress)
			}
			w.Flush()

			return buf, nil
		},
	},
}

var p2pCloseCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Stop listening for new connections to forward.",
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("all", "a", "Close all listeners."),
		cmdkit.StringOption("protocol", "p", "Match protocol name"),
		cmdkit.StringOption("listen-address", "l", "Match listen address"),
		cmdkit.StringOption("target-address", "t", "Match target address"),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		closeAll, _, _ := req.Option("all").Bool()
		protoOpt, p, _ := req.Option("protocol").String()
		listenOpt, l, _ := req.Option("listen-address").String()
		targetOpt, t, _ := req.Option("target-address").String()

		proto := protocol.ID(protoOpt)

		listen, err := ma.NewMultiaddr(listenOpt)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		target, err := ma.NewMultiaddr(targetOpt)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		if !(closeAll || p || l || t) {
			res.SetError(errors.New("no matching options given"), cmdkit.ErrNormal)
			return
		}

		if closeAll && (p || l || t) {
			res.SetError(errors.New("can't combine --all with other matching options"), cmdkit.ErrNormal)
			return
		}

		match := func(listener p2p.Listener) bool {
			if closeAll {
				return true
			}
			if p && proto != listener.Protocol() {
				return false
			}
			if l && !listen.Equal(listener.ListenAddress()) {
				return false
			}
			if t && !target.Equal(listener.TargetAddress()) {
				return false
			}
			return true
		}

		todo := make([]p2p.Listener, 0)
		n.P2P.Listeners.Lock()
		for _, l := range n.P2P.Listeners.Listeners {
			if !match(l) {
				continue
			}
			todo = append(todo, l)
		}
		n.P2P.Listeners.Unlock()

		var errs []string
		for _, l := range todo {
			if err := l.Close(); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) != 0 {
			res.SetError(fmt.Errorf("errors when closing streams: %s", strings.Join(errs, "; ")), cmdkit.ErrNormal)
			return
		}

		res.SetOutput(len(todo))
	},
	Type: int(0),
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) (io.Reader, error) {
			v, err := unwrapOutput(res.Output())
			if err != nil {
				return nil, err
			}

			closed := v.(int)
			buf := new(bytes.Buffer)
			fmt.Fprintf(buf, "Closed %d stream(s)\n", closed)

			return buf, nil
		},
	},
}

///////
// Listener
//

// p2pStreamCmd is the 'ipfs p2p stream' command
var p2pStreamCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "P2P stream management.",
		ShortDescription: "Create and manage p2p streams",
	},

	Subcommands: map[string]*cmds.Command{
		"ls":    p2pStreamLsCmd,
		"close": p2pStreamCloseCmd,
	},
}

var p2pStreamLsCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "List active p2p streams.",
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("headers", "v", "Print table headers (ID, Protocol, Local, Remote)."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		output := &P2PStreamsOutput{}

		for id, s := range n.P2P.Streams.Streams {
			output.Streams = append(output.Streams, P2PStreamInfoOutput{
				HandlerID: strconv.FormatUint(id, 10),

				Protocol: string(s.Protocol),

				OriginAddress: s.OriginAddr.String(),
				TargetAddress: s.TargetAddr.String(),
			})
		}

		res.SetOutput(output)
	},
	Type: P2PStreamsOutput{},
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) (io.Reader, error) {
			v, err := unwrapOutput(res.Output())
			if err != nil {
				return nil, err
			}

			headers, _, _ := res.Request().Option("headers").Bool()
			list := v.(*P2PStreamsOutput)
			buf := new(bytes.Buffer)
			w := tabwriter.NewWriter(buf, 1, 2, 1, ' ', 0)
			for _, stream := range list.Streams {
				if headers {
					fmt.Fprintln(w, "ID\tProtocol\tOrigin\tTarget")
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", stream.HandlerID, stream.Protocol, stream.OriginAddress, stream.TargetAddress)
			}
			w.Flush()

			return buf, nil
		},
	},
}

var p2pStreamCloseCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Close active p2p stream.",
	},
	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("id", false, false, "Stream identifier"),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("all", "a", "Close all streams."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		res.SetOutput(nil)

		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		closeAll, _, _ := req.Option("all").Bool()
		var handlerID uint64

		if !closeAll {
			if len(req.Arguments()) == 0 {
				res.SetError(errors.New("no id specified"), cmdkit.ErrNormal)
				return
			}

			handlerID, err = strconv.ParseUint(req.Arguments()[0], 10, 64)
			if err != nil {
				res.SetError(err, cmdkit.ErrNormal)
				return
			}
		}

		for id, stream := range n.P2P.Streams.Streams {
			if !closeAll && handlerID != id {
				continue
			}
			stream.Reset()
			if !closeAll {
				break
			}
		}
	},
}

func p2pGetNode(req cmds.Request) (*core.IpfsNode, error) {
	n, err := req.InvocContext().GetNode()
	if err != nil {
		return nil, err
	}

	config, err := n.Repo.Config()
	if err != nil {
		return nil, err
	}

	if !config.Experimental.Libp2pStreamMounting {
		return nil, errors.New("libp2p stream mounting not enabled")
	}

	if !n.OnlineMode() {
		return nil, ErrNotOnline
	}

	return n, nil
}
