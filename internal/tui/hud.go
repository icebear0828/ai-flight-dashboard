package tui

import (
	"fmt"
	"strings"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type periodCost struct {
	Label        string
	Cost         float64
	InputTokens  int
	CachedTokens int
	OutputTokens int
}

type Model struct {
	calc      *calculator.Calculator
	watch     *watcher.Watcher
	database  *db.DB
	usages    []watcher.TokenUsage
	totalCost float64
	periods   []periodCost
	quitting  bool
}

func NewModel(c *calculator.Calculator, w *watcher.Watcher, d *db.DB) Model {
	return Model{
		calc:     c,
		watch:    w,
		database: d,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.waitForUsage(), m.refreshPeriods())
}

type usageMsg watcher.TokenUsage
type periodsMsg []periodCost
type tickMsg struct{}

func (m Model) waitForUsage() tea.Cmd {
	return func() tea.Msg {
		u := <-m.watch.UsageChan
		return usageMsg(u)
	}
}

func (m Model) refreshPeriods() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return periodsMsg(nil)
		}
		now := time.Now().UTC()
		windows := []struct {
			label string
			dur   time.Duration
		}{
			{"1h", 1 * time.Hour},
			{"24h", 24 * time.Hour},
			{"7d", 7 * 24 * time.Hour},
			{"30d", 30 * 24 * time.Hour},
			{"3mo", 90 * 24 * time.Hour},
			{"6mo", 180 * 24 * time.Hour},
			{"1y", 365 * 24 * time.Hour},
		}

		var periods []periodCost
		for _, w := range windows {
			cost, in, ca, out, _ := m.database.QueryPeriodStatsSince(now.Add(-w.dur), m.watch.DeviceID)
			periods = append(periods, periodCost{Label: w.label, Cost: cost, InputTokens: in, CachedTokens: ca, OutputTokens: out})
		}
		total, tIn, tCa, tOut, _ := m.database.QueryPeriodStatsAll(m.watch.DeviceID)
		periods = append(periods, periodCost{Label: "ALL", Cost: total, InputTokens: tIn, CachedTokens: tCa, OutputTokens: tOut})

		return periodsMsg(periods)
	}
}

func (m Model) tickRefresh() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case usageMsg:
		u := watcher.TokenUsage(msg)
		m.usages = append(m.usages, u)
		cost, _ := m.calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.OutputTokens)
		m.totalCost += cost

		if m.database != nil {
			m.database.InsertUsage(u, cost, m.watch.DeviceID)
		}

		return m, tea.Batch(m.waitForUsage(), m.refreshPeriods())
	case periodsMsg:
		m.periods = []periodCost(msg)
		return m, m.tickRefresh()
	case tickMsg:
		return m, m.refreshPeriods()
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Shutting down AI Flight Dashboard...\n"
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	costStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Width(8)
	tableHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))

	var b strings.Builder
	b.WriteString(header.Render("✈️  AI Flight Dashboard") + "\n\n")

	// Time-window cost table
	if len(m.periods) > 0 {
		b.WriteString(tableHeader.Render(" Period   │ Cost (USD)") + "\n")
		b.WriteString(dimStyle.Render("──────────┼───────────") + "\n")
		for _, p := range m.periods {
			costStr := costStyle.Render(fmt.Sprintf("$%.4f", p.Cost))
			b.WriteString(fmt.Sprintf(" %s│ %s\n", labelStyle.Render(p.Label), costStr))
		}
		b.WriteString("\n")
	}

	// Live event counter
	b.WriteString(fmt.Sprintf(" Live Events: %d", len(m.usages)))
	if len(m.usages) > 0 {
		last := m.usages[len(m.usages)-1]
		b.WriteString(dimStyle.Render(fmt.Sprintf(" │ %s %s │ In:%d Ca:%d Out:%d",
			last.Source, last.Model, last.InputTokens, last.CachedTokens, last.OutputTokens)))
	}
	b.WriteString("\n")

	b.WriteString(dimStyle.Render("\n [q] quit │ refreshes every 5s") + "\n")
	return b.String()
}
