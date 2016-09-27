package icmp

import (
	"fmt"
	"strings"
	"time"

	ui "github.com/gizak/termui"
)

// Widgets represents termui widgets
type Widgets struct {
	Hops *ui.List
	ASN  *ui.List
	RTT  *ui.List
	Snt  *ui.List
	Pkl  *ui.List

	Menu   *ui.Par
	Header *ui.Par

	LCRTT *ui.LineChart
	BCPKL *ui.BarChart
}

// TermUI prints out trace loop by termui
func (i *Trace) TermUI() error {
	ui.DefaultEvtStream = ui.NewEvtStream()
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()

	uiTheme(i.uiTheme)

	var (
		done    = make(chan struct{})
		routers = make([]map[string]Stats, 65)
		stats   = make([]Stats, 65)

		w = initWidgets()

		rChanged bool
	)

	// init widgets parameters w/ trace info
	i.bridgeWidgetsTrace(w)

	// run loop trace route
	resp, err := i.MRun()
	if err != nil {
		return err
	}

	for i := 1; i < 65; i++ {
		routers[i] = make(map[string]Stats, 30)
	}

	screen1, screen2 := w.makeScreens()
	w.eventsHandler(done, screen1, screen2, stats)

	// init layout
	ui.Body.AddRows(screen1...)
	ui.Body.Align()
	ui.Render(ui.Body)

	go func() {
		var (
			hop, as, holder string
		)
	LOOP:
		for {
			select {
			case <-done:
				break LOOP
			case r, ok := <-resp:
				if !ok {
					break LOOP
				}

				if r.hop != "" {
					hop = r.hop
				} else {
					hop = r.ip
				}

				if r.whois.asn > 0 {
					as = fmt.Sprintf("%.0f", r.whois.asn)
					holder = strings.Fields(r.whois.holder)[0]
				} else {
					as = ""
					holder = ""
				}

				// statistics
				stats[r.num].count++
				w.Snt.Items[r.num] = fmt.Sprintf("%d", stats[r.num].count)

				router := routers[r.num][hop]
				router.count++

				if r.elapsed != 0 {

					// hop level statistics
					calcStatistics(&stats[r.num], r.elapsed)
					// router level statistics
					calcStatistics(&router, r.elapsed)
					// detect router changes
					rChanged = routerChange(hop, w.Hops.Items[r.num])

					w.Hops.Items[r.num] = fmt.Sprintf("[%-2d] %-45s", r.num, hop)
					w.ASN.Items[r.num] = fmt.Sprintf("%-6s %s", as, holder)
					w.RTT.Items[r.num] = fmt.Sprintf("%-6.2f\t%-6.2f\t%-6.2f\t%-6.2f", r.elapsed, stats[r.num].avg, stats[r.num].min, stats[r.num].max)

					if rChanged {
						w.Hops.Items[r.num] = termUICColor(w.Hops.Items[r.num], "fg-bold")
					}

					lcShift(r, w.LCRTT, ui.TermWidth())

				} else if w.Hops.Items[r.num] == "" {

					w.Hops.Items[r.num] = fmt.Sprintf("[%-2d] %-40s", r.num, "???")
					stats[r.num].pkl++
					router.pkl++

				} else if !strings.Contains(w.Hops.Items[r.num], "???") {

					hop = rmUIMetaData(w.Hops.Items[r.num])
					hop = fmt.Sprintf("[%-2d] %-45s", r.num, hop)
					w.Hops.Items[r.num] = termUICColor(hop, "fg-red")
					w.RTT.Items[r.num] = fmt.Sprintf("%-6.2s\t%-6.2f\t%-6.2f\t%-6.2f", "?", stats[r.num].avg, stats[r.num].min, stats[r.num].max)
					stats[r.num].pkl++
					router.pkl++

				} else {
					w.Hops.Items[r.num] = fmt.Sprintf("[%-2d] %-45s", r.num, "???")
					stats[r.num].pkl++
					router.pkl++

				}

				if len(w.BCPKL.DataLabels) > r.num-1 {
					w.BCPKL.DataLabels[r.num-1] = fmt.Sprintf("H%d", r.num)
					w.BCPKL.Data[r.num-1] = int(stats[r.num].pkl)
				} else {
					w.BCPKL.DataLabels = append(w.BCPKL.DataLabels, fmt.Sprintf("H%d", r.num))
					w.BCPKL.Data = append(w.BCPKL.Data, int(stats[r.num].pkl))

				}

				routers[r.num][hop] = router

				w.Pkl.Items[r.num] = fmt.Sprintf("%.1f", float64(stats[r.num].pkl)*100/float64(stats[r.num].count))
				ui.Render(ui.Body)
				// clean up in case of packet loss on the last hop at first try
				if r.last {
					for i := r.num + 1; i < 65; i++ {
						w.Hops.Items[i] = ""
					}
					//bc.DataLabels = bc.DataLabels[:r.num]
				}
			}
		}
		close(resp)
	}()

	ui.Loop()
	return nil
}

func (i *Trace) bridgeWidgetsTrace(w *Widgets) {
	// barchart
	w.LCRTT.BorderLabel = fmt.Sprintf("RTT: %s", i.host)
	// title
	t := fmt.Sprintf(
		"──[ myLG ]───────── traceroute to %s (%s), %d hops max",
		i.host,
		i.ip,
		i.maxTTL,
	)
	t += strings.Repeat(" ", ui.TermWidth()-len(t))
	w.Header.Text = t
}

// lcShift shifs line chart once it filled out
func lcShift(r HopResp, lc *ui.LineChart, width int) {
	if r.last {
		t := time.Now()
		lc.Data = append(lc.Data, r.elapsed)
		lc.DataLabels = append(lc.DataLabels, t.Format("04:05"))
		if len(lc.Data) > (ui.TermWidth()/2)-10 {
			lc.Data = lc.Data[1:]
			lc.DataLabels = lc.DataLabels[1:]
		}
	}
}

func rttWidget() *ui.LineChart {
	lc := ui.NewLineChart()
	lc.Height = 18
	lc.Mode = "dot"

	return lc
}

func pktLossWidget() *ui.BarChart {
	bc := ui.NewBarChart()
	bc.BorderLabel = "Packet Loss per hop"
	bc.Height = 18
	bc.TextColor = ui.ColorGreen
	bc.BarColor = ui.ColorRed
	bc.NumColor = ui.ColorYellow

	return bc
}

func headerWidget() *ui.Par {
	h := ui.NewPar("")
	h.Height = 1
	h.Width = ui.TermWidth()
	h.Y = 1
	h.TextBgColor = ui.ColorCyan
	h.TextFgColor = ui.ColorBlack
	h.Border = false

	return h
}

func menuWidget() *ui.Par {
	var items = []string{
		"Press [q] to quit",
		"[r] to reset statistics",
		"[1,2] to change display mode",
	}

	m := ui.NewPar(strings.Join(items, ", "))
	m.Height = 1
	m.Width = 20
	m.Y = 1
	m.Border = false

	return m
}

func (w *Widgets) eventsHandler(done chan struct{}, s1, s2 []*ui.Row, stats []Stats) {
	// exit
	ui.Handle("/sys/kbd/q", func(ui.Event) {
		done <- struct{}{}
		ui.StopLoop()
	})

	// change display mode to one
	ui.Handle("/sys/kbd/1", func(e ui.Event) {
		ui.Clear()
		ui.Body.Rows = ui.Body.Rows[:0]
		ui.Body.AddRows(s1...)
		ui.Body.Align()
		ui.Render(ui.Body)
	})

	// change display mode to two
	ui.Handle("/sys/kbd/2", func(e ui.Event) {
		ui.Clear()
		ui.Body.Rows = ui.Body.Rows[:0]
		ui.Body.AddRows(s2...)
		ui.Body.Align()
		ui.Render(ui.Body)
	})

	// resize
	ui.Handle("/sys/wnd/resize", func(e ui.Event) {
		ui.Body.Width = ui.TermWidth()
		ui.Body.Align()
		ui.Render(ui.Body)
	})

	// reset statistics and display
	ui.Handle("/sys/kbd/r", func(ui.Event) {
		for i := 1; i < 65; i++ {
			w.Hops.Items[i] = ""
			w.ASN.Items[i] = ""
			w.RTT.Items[i] = ""
			w.Snt.Items[i] = ""
			w.Pkl.Items[i] = ""

			stats[i].count = 0
			stats[i].avg = 0
			stats[i].min = 0
			stats[i].max = 0
			stats[i].pkl = 0
		}
		w.LCRTT.Data = w.LCRTT.Data[:0]
		w.LCRTT.DataLabels = w.LCRTT.DataLabels[:0]
	})

}

func (w *Widgets) makeScreens() ([]*ui.Row, []*ui.Row) {
	// screens1 - trace statistics
	screen1 := []*ui.Row{
		ui.NewRow(
			ui.NewCol(12, 0, w.Header),
		),
		ui.NewRow(
			ui.NewCol(12, 0, w.Menu),
		),
		ui.NewRow(
			ui.NewCol(5, 0, w.Hops),
			ui.NewCol(2, 0, w.ASN),
			ui.NewCol(1, 0, w.Pkl),
			ui.NewCol(1, 0, w.Snt),
			ui.NewCol(3, 0, w.RTT),
		),
	}
	// screen2 - trace line chart
	screen2 := []*ui.Row{
		ui.NewRow(
			ui.NewCol(12, 0, w.Header),
		),
		ui.NewRow(
			ui.NewCol(12, 0, w.Menu),
		),
		ui.NewRow(
			ui.NewCol(6, 0, w.LCRTT),
		),
		ui.NewRow(
			ui.NewCol(6, 0, w.BCPKL),
		),
	}

	return screen1, screen2
}

func initWidgets() *Widgets {
	var (
		hops = ui.NewList()
		asn  = ui.NewList()
		rtt  = ui.NewList()
		snt  = ui.NewList()
		pkl  = ui.NewList()

		lists = []*ui.List{hops, asn, rtt, snt, pkl}
	)

	for _, l := range lists {
		l.Items = make([]string, 65)
		l.X = 0
		l.Y = 0
		l.Height = 35
		l.Border = false
	}

	// title
	hops.Items[0] = fmt.Sprintf("[%-50s](fg-bold)", "Host")
	asn.Items[0] = fmt.Sprintf("[ %-6s %-6s](fg-bold)", "ASN", "Holder")
	rtt.Items[0] = fmt.Sprintf("[%-6s %-6s %-6s %-6s](fg-bold)", "Last", "Avg", "Best", "Wrst")
	snt.Items[0] = "[Sent](fg-bold)"
	pkl.Items[0] = "[Loss%](fg-bold)"

	return &Widgets{
		Hops: hops,
		ASN:  asn,
		RTT:  rtt,
		Snt:  snt,
		Pkl:  pkl,

		Menu:   menuWidget(),
		Header: headerWidget(),
		LCRTT:  rttWidget(),
		BCPKL:  pktLossWidget(),
	}
}

func uiTheme(t string) {

	switch t {
	case "light":
		ui.ColorMap["bg"] = ui.ColorWhite
		ui.ColorMap["fg"] = ui.ColorBlack
		ui.ColorMap["label.fg"] = ui.ColorBlack | ui.AttrBold
		ui.ColorMap["linechart.axes.fg"] = ui.ColorBlack
		ui.ColorMap["linechart.line.fg"] = ui.ColorGreen
		ui.ColorMap["border.fg"] = ui.ColorBlue
	default:
		// dark theme
		ui.ColorMap["bg"] = ui.ColorBlack
		ui.ColorMap["fg"] = ui.ColorWhite
		ui.ColorMap["label.fg"] = ui.ColorWhite | ui.AttrBold
		ui.ColorMap["linechart.axes.fg"] = ui.ColorWhite
		ui.ColorMap["linechart.line.fg"] = ui.ColorGreen
		ui.ColorMap["border.fg"] = ui.ColorCyan
	}
	ui.Clear()
}
