package dispatcher

import (
	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/features/stats"
)

type SizeStatWriter struct {
	Counter stats.Counter
	Writer  buf.Writer
}

func (w *SizeStatWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.Counter.Add(int64(mb.Len()))
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *SizeStatWriter) Close() error {
	return common.Close(w.Writer)
}

func (w *SizeStatWriter) Interrupt() {
	common.Interrupt(w.Writer)
}

type PerIPStatWriter struct {
	Address string
	Uplink  bool
	Mapper  stats.Mapper
	Writer  buf.Writer
}

func (w *PerIPStatWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if w.Uplink {
		w.Mapper.Add(w.Address, int64(mb.Len()), 0)
	} else {
		w.Mapper.Add(w.Address, 0, int64(mb.Len()))
	}
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *PerIPStatWriter) Close() error {
	return common.Close(w.Writer)
}

func (w *PerIPStatWriter) Interrupt() {
	common.Interrupt(w.Writer)
}
