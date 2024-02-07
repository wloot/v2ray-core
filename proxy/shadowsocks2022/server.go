package shadowsocks2022

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	singAuth "github.com/sagernet/sing/common/auth"
	singExceptions "github.com/sagernet/sing/common/exceptions"
	singMetadata "github.com/sagernet/sing/common/metadata"
	singNetwork "github.com/sagernet/sing/common/network"
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
	v := core.MustFromContext(ctx)
	inbound := &Inbound{
		policyManager: v.GetFeature(policy.ManagerType()).(policy.Manager),
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
				email = strings.ToLower(mUser.Email)
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
		email = strings.ToLower(u.Email)
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
	return []net.Network{net.Network_TCP}
}

func (i *Inbound) Process(ctx context.Context, network net.Network, connection internet.Connection, dispatcher routing.Dispatcher) error {
	inbound := session.InboundFromContext(ctx)

	var metadata singMetadata.Metadata
	if inbound.Source.IsValid() {
		metadata.Source = singMetadata.ParseSocksaddr(inbound.Source.NetAddr())
	}

	ctx = contextWithDispatcher(ctx, dispatcher)
	if network == net.Network_TCP {
		return returnError(i.service.NewConnection(ctx, connection, metadata))
	}
	return nil
}

func (i *Inbound) NewConnection(ctx context.Context, conn net.Conn, metadata singMetadata.Metadata) error {
	inbound := session.InboundFromContext(ctx)
	email, _ := singAuth.UserFromContext[string](ctx)
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

func (i *Inbound) NewPacketConnection(ctx context.Context, conn singNetwork.PacketConn, metadata singMetadata.Metadata) error {
	return nil
}

func (i *Inbound) NewError(ctx context.Context, err error) {
	if singExceptions.IsClosed(err) {
		return
	}
	newError(err).AtWarning().WriteToLog()
}

func returnError(err error) error {
	if singExceptions.IsClosed(err) {
		return nil
	}
	return err
}

func toDestination(socksaddr singMetadata.Socksaddr, network net.Network) net.Destination {
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
