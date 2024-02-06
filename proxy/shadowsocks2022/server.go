package shadowsocks2022

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	A "github.com/sagernet/sing/common/auth"
	B "github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/log"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/session"
	"github.com/v2fly/v2ray-core/v5/common/signal"
	"github.com/v2fly/v2ray-core/v5/common/task"
	"github.com/v2fly/v2ray-core/v5/features/policy"
	"github.com/v2fly/v2ray-core/v5/features/routing"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

// MemoryAccount is an account type converted from Account.
type MemoryAccount struct {
	Key string
}

// Equals implements protocol.Account.Equals().
func (a *MemoryAccount) Equals(another protocol.Account) bool {
	if account, ok := another.(*MemoryAccount); ok {
		return a.Key == account.Key
	}
	return false
}

// AsAccount implements protocol.AsAccount.
func (u *Account) AsAccount() (protocol.Account, error) {
	return &MemoryAccount{
		Key: u.GetKey(),
	}, nil
}

func init() {
	common.Must(common.RegisterConfig((*ServerConfig)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		return NewMultiServer(ctx, config.(*ServerConfig))
	}))
}

type Inbound struct {
	sync.Mutex
	networks      []net.Network
	policyManager policy.Manager
	users         map[string]*protocol.MemoryUser
	service       *shadowaead_2022.MultiService[string]
}

func (i *Inbound) updateServiceWithUsers() error {
	userList := make([]string, 0, len(i.users))
	keyList := make([][]byte, 0, len(i.users))
	for email := range i.users {
		key, err := base64.StdEncoding.DecodeString(i.users[email].Account.(*MemoryAccount).Key)
		if err != nil {
			return newError("failed to decode user key").Base(err)
		}
		keyList = append(keyList, key)
		userList = append(userList, email)
	}
	return i.service.UpdateUsers(userList, keyList)
}

func NewMultiServer(ctx context.Context, config *ServerConfig) (*Inbound, error) {
	networks := config.Network
	if len(networks) == 0 {
		networks = []net.Network{
			net.Network_TCP,
			net.Network_UDP,
		}
	}
	v := core.MustFromContext(ctx)
	inbound := &Inbound{
		policyManager: v.GetFeature(policy.ManagerType()).(policy.Manager),
		networks:      networks,
		users:         make(map[string]*protocol.MemoryUser),
	}

	service, err := shadowaead_2022.NewMultiServiceWithPassword[string](config.Method, config.Key, 300, inbound, nil)
	if err != nil {
		return nil, newError("failed to create service").Base(err)
	}
	if len(config.User) > 0 {
		for _, user := range config.User {
			mUser, err := user.ToMemoryUser()
			if err != nil {
				return nil, newError("failed to get user").Base(err)
			}
			var email string
			if mUser.Email != "" {
				mUser.Email = strings.ToLower(mUser.Email)
				email = mUser.Email
			} else {
				email = config.Key
			}
			inbound.users[email] = mUser
		}
		err = inbound.updateServiceWithUsers()
		if err != nil {
			return nil, newError("failed to add users").Base(err)
		}
	}

	inbound.service = service
	return inbound, nil
}

// AddUser implements proxy.UserManager.AddUser().
func (i *Inbound) AddUser(ctx context.Context, u *protocol.MemoryUser) error {
	i.Lock()
	defer i.Unlock()

	var email string
	if u.Email != "" {
		u.Email = strings.ToLower(u.Email)
		email = u.Email
		if _, found := i.users[email]; found {
			return newError("User ", u.Email, " already exists.")
		}
	} else {
		email = u.Account.(*MemoryAccount).Key
	}
	i.users[email] = u

	return i.updateServiceWithUsers()
}

// RemoveUser implements proxy.UserManager.RemoveUser().
func (i *Inbound) RemoveUser(ctx context.Context, email string) error {
	if email == "" {
		return newError("Email must not be empty.")
	}
	email = strings.ToLower(email)
	i.Lock()
	defer i.Unlock()

	_, found := i.users[email]
	if !found {
		return newError("User ", email, " not found.")
	}
	delete(i.users, email)

	return i.updateServiceWithUsers()
}

func (i *Inbound) Network() []net.Network {
	return i.networks
}

func (i *Inbound) Process(ctx context.Context, network net.Network, connection internet.Connection, dispatcher routing.Dispatcher) error {
	inbound := session.InboundFromContext(ctx)

	var metadata M.Metadata
	if inbound.Source.IsValid() {
		metadata.Source = M.ParseSocksaddr(inbound.Source.NetAddr())
	}

	ctx = contextWithDispatcher(ctx, dispatcher)
	if network == net.Network_TCP {
		return returnError(i.service.NewConnection(ctx, connection, metadata))
	} else {
		reader := buf.NewReader(connection)
		pc := &natPacketConn{connection}
		for {
			mb, err := reader.ReadMultiBuffer()
			if err != nil {
				buf.ReleaseMulti(mb)
				return returnError(err)
			}
			for _, buffer := range mb {
				packet := B.As(buffer.Bytes()).ToOwned()
				buffer.Release()
				err = i.service.NewPacket(ctx, pc, packet, metadata)
				if err != nil {
					packet.Release()
					buf.ReleaseMulti(mb)
					return err
				}
			}
		}
	}
}

func (i *Inbound) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	inbound := session.InboundFromContext(ctx)
	email, _ := A.UserFromContext[string](ctx)
	user, found := i.users[email]
	if !found {
		user = &protocol.MemoryUser{}
	}
	inbound.User = user
	ctx = log.ContextWithAccessMessage(ctx, &log.AccessMessage{
		From:   metadata.Source,
		To:     metadata.Destination,
		Status: log.AccessAccepted,
		Email:  user.Email,
	})
	newError("tunnelling request to tcp:", metadata.Destination).WriteToLog(session.ExportIDToError(ctx))
	dispatcher := dispatcherFromContext(ctx)
	destination := toDestination(metadata.Destination, net.Network_TCP)
	if !destination.IsValid() {
		return newError("invalid destination")
	}
	sessionPolicy := i.policyManager.ForLevel(user.Level)
	return handleConnection(ctx, sessionPolicy, destination, buf.NewReader(conn), buf.NewWriter(conn), dispatcher)
}

func (i *Inbound) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	inbound := session.InboundFromContext(ctx)
	email, _ := A.UserFromContext[string](ctx)
	user, found := i.users[email]
	if !found {
		user = &protocol.MemoryUser{}
	}
	inbound.User = user
	ctx = log.ContextWithAccessMessage(ctx, &log.AccessMessage{
		From:   metadata.Source,
		To:     metadata.Destination,
		Status: log.AccessAccepted,
		Email:  user.Email,
	})
	newError("tunnelling request to udp:", metadata.Destination).WriteToLog(session.ExportIDToError(ctx))
	dispatcher := dispatcherFromContext(ctx)
	destination := toDestination(metadata.Destination, net.Network_UDP)
	link, err := dispatcher.Dispatch(ctx, destination)
	if err != nil {
		return err
	}
	outConn := &packetConnWrapper{
		Reader: link.Reader,
		Writer: link.Writer,
		Dest:   destination,
	}
	return bufio.CopyPacketConn(ctx, conn, outConn)
}

func (i *Inbound) NewError(ctx context.Context, err error) {
	if E.IsClosed(err) {
		return
	}
	newError(err).AtWarning().WriteToLog()
}

type natPacketConn struct {
	net.Conn
}

func (c *natPacketConn) ReadPacket(buffer *B.Buffer) (addr M.Socksaddr, err error) {
	_, err = buffer.ReadFrom(c)
	return
}

func (c *natPacketConn) WritePacket(buffer *B.Buffer, addr M.Socksaddr) error {
	_, err := buffer.WriteTo(c)
	return err
}

func toDestination(socksaddr M.Socksaddr, network net.Network) net.Destination {
	if socksaddr.IsFqdn() {
		return net.Destination{
			Network: network,
			Address: net.DomainAddress(socksaddr.Fqdn),
			Port:    net.Port(socksaddr.Port),
		}
	}
	if socksaddr.IsIP() {
		return net.Destination{
			Network: network,
			Address: net.IPAddress(socksaddr.Addr.AsSlice()),
			Port:    net.Port(socksaddr.Port),
		}
	}
	return net.Destination{}
}

func toSocksaddr(destination net.Destination) M.Socksaddr {
	var addr M.Socksaddr
	switch destination.Address.Family() {
	case net.AddressFamilyDomain:
		addr.Fqdn = destination.Address.Domain()
	default:
		addr.Addr = M.AddrFromIP(destination.Address.IP())
	}
	addr.Port = uint16(destination.Port)
	return addr
}

type packetConnWrapper struct {
	buf.Reader
	buf.Writer
	net.Conn
	Dest   net.Destination
	cached buf.MultiBuffer
}

func (w *packetConnWrapper) ReadPacket(buffer *B.Buffer) (M.Socksaddr, error) {
	if w.cached != nil {
		mb, bb := buf.SplitFirst(w.cached)
		if bb == nil {
			w.cached = nil
		} else {
			buffer.Write(bb.Bytes())
			w.cached = mb
			destination := w.Dest
			if bb.UDP != nil {
				destination.Address = net.ParseAddress(bb.UDP.Addr)
				destination.Port = net.Port(bb.UDP.Port)
			}
			bb.Release()
			return toSocksaddr(destination), nil
		}
	}
	mb, err := w.ReadMultiBuffer()
	if err != nil {
		return M.Socksaddr{}, err
	}
	nb, bb := buf.SplitFirst(mb)
	if bb == nil {
		return M.Socksaddr{}, nil
	} else {
		buffer.Write(bb.Bytes())
		w.cached = nb
		destination := w.Dest
		if bb.UDP != nil {
			destination.Address = net.ParseAddress(bb.UDP.Addr)
			destination.Port = net.Port(bb.UDP.Port)
		}
		bb.Release()
		return toSocksaddr(destination), nil
	}
}

func (w *packetConnWrapper) WritePacket(buffer *B.Buffer, destination M.Socksaddr) error {
	vBuf := buf.New()
	vBuf.Write(buffer.Bytes())

	vBuf.UDP = &buf.UDP{
		Addr: destination.Addr.String(),
		Port: destination.Port,
	}
	return w.Writer.WriteMultiBuffer(buf.MultiBuffer{vBuf})
}

func (w *packetConnWrapper) Close() error {
	buf.ReleaseMulti(w.cached)
	return nil
}

func returnError(err error) error {
	if E.IsClosed(err) {
		return nil
	}
	return err
}

func handleConnection(ctx context.Context, sessionPolicy policy.Session,
	destination net.Destination,
	clientReader buf.Reader,
	clientWriter buf.Writer, dispatcher routing.Dispatcher,
) error {
	ctx, cancel := context.WithCancel(ctx)
	timer := signal.CancelAfterInactivity(ctx, cancel, sessionPolicy.Timeouts.ConnectionIdle)
	ctx = policy.ContextWithBufferPolicy(ctx, sessionPolicy.Buffer)

	link, err := dispatcher.Dispatch(ctx, destination)
	if err != nil {
		return newError("failed to dispatch request to ", destination).Base(err)
	}

	requestDone := func() error {
		defer timer.SetTimeout(sessionPolicy.Timeouts.DownlinkOnly)

		if err := buf.Copy(clientReader, link.Writer, buf.UpdateActivity(timer)); err != nil {
			return newError("failed to transfer request").Base(err)
		}
		return nil
	}

	responseDone := func() error {
		defer timer.SetTimeout(sessionPolicy.Timeouts.UplinkOnly)

		if err := buf.Copy(link.Reader, clientWriter, buf.UpdateActivity(timer)); err != nil {
			return newError("failed to write response").Base(err)
		}
		return nil
	}

	requestDonePost := task.OnSuccess(requestDone, task.Close(link.Writer))
	if err := task.Run(ctx, requestDonePost, responseDone); err != nil {
		common.Must(common.Interrupt(link.Reader))
		common.Must(common.Interrupt(link.Writer))
	}
	return nil
}

type dispatcherKey struct{}

func contextWithDispatcher(ctx context.Context, dispatcher routing.Dispatcher) context.Context {
	return context.WithValue(ctx, dispatcherKey{}, dispatcher)
}

func dispatcherFromContext(ctx context.Context) routing.Dispatcher {
	if dispatcher, ok := ctx.Value(dispatcherKey{}).(routing.Dispatcher); ok {
		return dispatcher
	}
	return nil
}
