package dispatcher

import (
	"context"
	"time"

	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/features/stats"
	"golang.org/x/time/rate"
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

type LimitWriter struct {
	Limit  *rate.Limiter
	Writer buf.Writer
}

func (w *LimitWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if w.Limit.AllowN(time.Now(), int(mb.Len())) == true {
		return w.Writer.WriteMultiBuffer(mb)
	}
	var b *buf.Buffer
	for {
		mb, b = buf.SplitFirst(mb)
		if b == nil {
			break
		}
		w.Limit.WaitN(context.Background(), int(b.Len()))
		err := w.Writer.WriteMultiBuffer(buf.MultiBuffer{b})
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *LimitWriter) Close() error {
	return common.Close(w.Writer)
}

func (w *LimitWriter) Interrupt() {
	common.Interrupt(w.Writer)
}
