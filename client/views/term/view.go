// interactive terminal interface for local clients
package term

import (
	"time"
	"zach-rock/client/mvc"
	"zach-rock/log"
	"zach-rock/proto"
	"zach-rock/util"

	termbox "github.com/nsf/termbox-go"
)

type TermView struct {
	ctl      mvc.Controller
	updates  chan interface{}
	flush    chan int
	shutdown chan int
	redraw   *util.Broadcast
	// subviews []mvc.View
	log.Logger
	*area
}

func NewTermView(ctl mvc.Controller) *TermView {
	// initialize terminal display
	err := termbox.Init()
	if err != nil {
		return nil
	}

	w, _ := termbox.Size()

	v := &TermView{
		ctl:      ctl,
		updates:  ctl.Updates().Reg(),
		redraw:   util.NewBroadcast(),
		flush:    make(chan int),
		shutdown: make(chan int),
		Logger:   log.NewPrefixLogger("view", "term"),
		area:     NewArea(0, 0, w, 10),
	}

	ctl.Go(v.run)
	ctl.Go(v.input)

	return v
}

func connStatusRepr(status mvc.ConnStatus) (string, termbox.Attribute) {
	switch status {
	case mvc.ConnConnecting:
		return "connecting", termbox.ColorCyan
	case mvc.ConnReconnecting:
		return "reconnecting", termbox.ColorRed
	case mvc.ConnOnline:
		return "online", termbox.ColorGreen
	}
	return "unknown", termbox.ColorWhite
}

func (v *TermView) draw() {
	state := v.ctl.State()

	v.Clear()

	// quit instructions
	quitMsg := "(Ctrl+C to quit)"
	v.Printf(v.w-len(quitMsg), 0, quitMsg)

	// new version message
	updateStatus := state.GetUpdateStatus()
	var updateMsg string
	switch updateStatus {
	case mvc.UpdateNone:
		updateMsg = ""
	case mvc.UpdateInstalling:
		updateMsg = "client is updating"
	case mvc.UpdateReady:
		updateMsg = "client has updated: restart client for the new version"
	case mvc.UpdateAvailable:
		updateMsg = "new version available at future"
	default:
		pct := float64(updateStatus) / 100.0
		const barLength = 25
		full := int(barLength * pct)
		bar := make([]byte, barLength+2)
		bar[0] = '['
		bar[barLength+1] = ']'
		for i := 0; i < 25; i++ {
			if i <= full {
				bar[i+1] = '#'
			} else {
				bar[i+1] = ' '
			}
		}
		updateMsg = "Downloading update: " + string(bar)
	}

	if updateMsg != "" {
		v.APrintf(termbox.ColorYellow, 30, 0, updateMsg)
	}

	v.APrintf(termbox.ColorLightMagenta|termbox.AttrBold, 0, 1, "Let's rock!	------------------------ Zach.Lu")
	statusStr, statusColor := connStatusRepr(state.GetConnStatus())
	v.APrintf(statusColor, 0, 3, "%-30s%s", "Tunnel Status", statusStr)

	v.Printf(0, 4, "%-30s%s/%s", "Version", state.GetClientVersion(), state.GetServerVersion())
	var i int = 5
	for _, t := range state.GetTunnels() {
		v.Printf(0, i, "%-30s%s -> %s", "Forwarding", t.PublicUrl, t.LocalAddr)
		i++
	}
	// v.Printf(0, i+0, "%-30s%s", "Web Interface", v.ctl.GetWebInspectAddr())

	_, connTimer := state.GetConnectionMetrics()
	// v.Printf(0, i+1, "%-30s%d", "# Conn", connMeter.Count())

	msec := float64(time.Millisecond)
	v.Printf(0, i+2, "%-30s%.2fms", "Avg Conn Time", connTimer.Mean()/msec)

	err := termbox.Flush()

	if err != nil {
		return 
	}
}

func (v *TermView) run() {
	defer close(v.shutdown)
	defer termbox.Close()

	redraw := v.redraw.Reg()
	defer v.redraw.UnReg(redraw)

	v.draw()
	for {
		v.Debug("Waiting for update")
		select {
		case <-v.flush:
			err := termbox.Flush()
			if err != nil {
				return
			}

		case <-v.updates:
			v.draw()

		case <-redraw:
			v.draw()

		case <-v.shutdown:
			return
		}
	}
}

func (v *TermView) Shutdown() {
	v.shutdown <- 1
	<-v.shutdown
}

func (v *TermView) Flush() {
	v.flush <- 1
}

func (v *TermView) NewHttpView(p *proto.Http) *HttpView {
	return newTermHttpView(v.ctl, v, p, 0, 12)
}

func (v *TermView) input() {
	for {
		ev := termbox.PollEvent()
		switch ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlC:
				v.Info("Got quit command")
				v.ctl.Shutdown("")
			}

		case termbox.EventResize:
			v.Info("Resize event, redrawing")
			v.redraw.In() <- 1

		case termbox.EventError:
			panic(ev.Err)
		}
	}
}
