package mux

import (
	"io"

	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/crypto"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/serial"
)

// PacketReader is an io.Reader that reads whole chunk of Mux frames every time.
type PacketReader struct {
	reader io.Reader
	eof    bool
	dest   *net.Destination
}

// NewPacketReader creates a new PacketReader.
func NewPacketReader(reader io.Reader, dest *net.Destination) *PacketReader {
	return &PacketReader{
		reader: reader,
		eof:    false,
		dest:   dest,
	}
}

// ReadMultiBuffer implements buf.Reader.
func (r *PacketReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	if r.eof {
		return nil, io.EOF
	}

	size, err := serial.ReadUint16(r.reader)
	if err != nil {
		return nil, err
	}

	if size > buf.Size {
		return nil, newError("packet size too large: ", size)
	}

	b := buf.New()
	if _, err := b.ReadFullFrom(r.reader, int32(size)); err != nil {
		b.Release()
		return nil, err
	}
	r.eof = true
	if r.dest != nil && r.dest.Network == net.Network_UDP {
		b.UDP = &buf.UDP{
			Addr: r.dest.Address.String(),
			Port: uint16(r.dest.Port),
		}
	}
	return buf.MultiBuffer{b}, nil
}

// NewStreamReader creates a new StreamReader.
func NewStreamReader(reader *buf.BufferedReader) buf.Reader {
	return crypto.NewChunkStreamReaderWithChunkCount(crypto.PlainChunkSizeParser{}, reader, 1)
}
