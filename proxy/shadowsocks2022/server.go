package shadowsocks2022

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	A "github.com/sagernet/sing/common/auth"
	B "github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/log"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/session"
	"github.com/v2fly/v2ray-core/v5/common/uuid"
	"github.com/v2fly/v2ray-core/v5/features/routing"
	"github.com/v2fly/v2ray-core/v5/transport"
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
	users    []*protocol.MemoryUser
	networks []net.Network
	service  *shadowaead_2022.MultiService[int]
}

func (i *Inbound) updateServiceWithUsers() error {
	userList := make([]int, 0, len(i.users))
	passwordList := make([]string, 0, len(i.users))
	for index := range i.users {
		passwordList = append(passwordList, i.users[index].Account.(*MemoryAccount).Key)
		userList = append(userList, index)
	}
	return i.service.UpdateUsersWithPasswords(userList, passwordList)
}

func NewMultiServer(ctx context.Context, config *ServerConfig) (*Inbound, error) {
	networks := config.Network
	if len(networks) == 0 {
		networks = []net.Network{
			net.Network_TCP,
			net.Network_UDP,
		}
	}
	inbound := &Inbound{
		networks: networks,
	}

	service, err := shadowaead_2022.NewMultiServiceWithPassword[int](config.Method, config.Key, 300, inbound, nil)
	if err != nil {
		return nil, newError("create service").Base(err)
	}
	if len(config.User) > 0 {
		for i, user := range config.User {
			mUser, err := user.ToMemoryUser()
			if err != nil {
				return nil, newError("failed to get user").Base(err)
			}
			if mUser.Email == "" {
				u := uuid.New()
				user.Email = "unnamed-user-" + strconv.Itoa(i) + "-" + u.String()
			}
			inbound.users = append(inbound.users, mUser)
		}
		err = inbound.updateServiceWithUsers()
		if err != nil {
			return nil, newError("create service").Base(err)
		}
	}

	inbound.service = service
	return inbound, nil
}

// AddUser implements proxy.UserManager.AddUser().
func (i *Inbound) AddUser(ctx context.Context, u *protocol.MemoryUser) error {
	i.Lock()
	defer i.Unlock()

	for idx := range i.users {
		if i.users[idx].Account.Equals(u.Account) || i.users[idx].Email == u.Email {
			return newError("User ", u.Email, " already exists.")
		}
	}
	i.users = append(i.users, u)

	return i.updateServiceWithUsers()
}

// RemoveUser implements proxy.UserManager.RemoveUser().
func (i *Inbound) RemoveUser(ctx context.Context, email string) error {
	if email == "" {
		return newError("Email must not be empty.")
	}

	i.Lock()
	defer i.Unlock()

	idx := -1
	for ii, u := range i.users {
		if strings.EqualFold(u.Email, email) {
			idx = ii
			break
		}
	}

	if idx == -1 {
		return newError("User ", email, " not found.")
	}

	ulen := len(i.users)

	i.users[idx] = i.users[ulen-1]
	i.users[ulen-1] = nil
	i.users = i.users[:ulen-1]

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
	userInt, _ := A.UserFromContext[int](ctx)
	user := i.users[userInt]
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

	link, err := dispatcher.Dispatch(ctx, destination)
	if err != nil {
		return err
	}
	return copyConn(ctx, conn, link, conn)
}

func (i *Inbound) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	inbound := session.InboundFromContext(ctx)
	userInt, _ := A.UserFromContext[int](ctx)
	user := i.users[userInt]
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
	// IsFqdn() implicitly checks if the domain name is valid
	if socksaddr.IsFqdn() {
		return net.Destination{
			Network: network,
			Address: net.DomainAddress(socksaddr.Fqdn),
			Port:    net.Port(socksaddr.Port),
		}
	}

	// IsIP() implicitly checks if the IP address is valid
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

func copyConn(ctx context.Context, inboundConn net.Conn, link *transport.Link, serverConn net.Conn) error {
	conn := &pipeConnWrapper{
		W:    link.Writer,
		Conn: inboundConn,
	}
	if ir, ok := link.Reader.(io.Reader); ok {
		conn.R = ir
	} else {
		conn.R = &buf.BufferedReader{Reader: link.Reader}
	}
	return returnError(bufio.CopyConn(ctx, conn, serverConn))
}

type pipeConnWrapper struct {
	R io.Reader
	W buf.Writer
	net.Conn
}

func (w *pipeConnWrapper) Close() error {
	return nil
}

func (w *pipeConnWrapper) Read(b []byte) (n int, err error) {
	return w.R.Read(b)
}

func (w *pipeConnWrapper) Write(p []byte) (n int, err error) {
	n = len(p)
	var mb buf.MultiBuffer
	pLen := len(p)
	for pLen > 0 {
		buffer := buf.New()
		if pLen > buf.Size {
			_, err = buffer.Write(p[:buf.Size])
			p = p[buf.Size:]
		} else {
			buffer.Write(p)
		}
		pLen -= int(buffer.Len())
		mb = append(mb, buffer)
	}
	err = w.W.WriteMultiBuffer(mb)
	if err != nil {
		n = 0
		buf.ReleaseMulti(mb)
	}
	return
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
