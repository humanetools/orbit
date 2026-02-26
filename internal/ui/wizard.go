package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
)

// Wizard phases
const (
	phaseWelcome        = iota
	phasePlatformSelect // multi-select platforms to connect
	phaseTokenInput     // enter token for current platform
	phaseTokenValidate  // async validation + discovery
	phaseProjectName    // enter project name
	phaseServiceSelect  // multi-select discovered services
	phaseSaving         // async save
	phaseDone           // show summary and exit
)

// --- Messages ---

type tokenValidatedMsg struct {
	platform string
	err      error
	services []platform.DiscoveredService
}

type configSavedMsg struct {
	err error
}

// --- Model ---

// WizardModel is the Bubbletea model for the orbit init wizard.
type WizardModel struct {
	phase int

	// Platform selection
	platforms        []string // all available platform names (sorted)
	platformCursor   int
	platformSelected map[int]bool // indices of selected platforms

	// Token input — iterate through selected platforms one at a time
	selectedPlatforms []string          // ordered list of platforms to connect
	currentPlatIdx    int               // which platform we're currently entering a token for
	tokenInput        textinput.Model   // shared text input for tokens
	rawTokens         map[string]string // platform → plaintext token (in memory only)
	validationErr     string            // error from last validation

	// Project name
	projectInput textinput.Model

	// Discovered services
	allServices     []platform.DiscoveredService
	discoveryErrors map[string]error
	serviceCursor   int
	serviceSelected map[int]bool

	// Saving
	savedProject string
	saveErr      string

	// General
	quitting bool
	width    int
	height   int
}

// NewWizardModel creates the initial wizard model.
func NewWizardModel() WizardModel {
	names := platform.Names()
	sort.Strings(names)

	ti := textinput.New()
	ti.Placeholder = "paste token here"
	ti.EchoMode = textinput.EchoNone
	ti.CharLimit = 256
	ti.Width = 60

	pi := textinput.New()
	pi.Placeholder = "my-project"
	pi.CharLimit = 64
	pi.Width = 40

	return WizardModel{
		phase:            phaseWelcome,
		platforms:        names,
		platformSelected: make(map[int]bool),
		rawTokens:        make(map[string]string),
		tokenInput:       ti,
		projectInput:     pi,
		serviceSelected:  make(map[int]bool),
		discoveryErrors:  make(map[string]error),
	}
}

// Init satisfies tea.Model.
func (m WizardModel) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model.
func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global quit
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}

	case tokenValidatedMsg:
		return m.handleTokenValidated(msg)

	case configSavedMsg:
		return m.handleConfigSaved(msg)
	}

	switch m.phase {
	case phaseWelcome:
		return m.updateWelcome(msg)
	case phasePlatformSelect:
		return m.updatePlatformSelect(msg)
	case phaseTokenInput:
		return m.updateTokenInput(msg)
	case phaseTokenValidate:
		// Ignore key events while validating (except ctrl+c handled above)
		return m, nil
	case phaseProjectName:
		return m.updateProjectName(msg)
	case phaseServiceSelect:
		return m.updateServiceSelect(msg)
	case phaseSaving:
		return m, nil
	case phaseDone:
		return m.updateDone(msg)
	}

	return m, nil
}

// --- Phase update handlers ---

func (m WizardModel) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		m.phase = phasePlatformSelect
	}
	return m, nil
}

func (m WizardModel) updatePlatformSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		if m.platformCursor > 0 {
			m.platformCursor--
		}
	case tea.KeyDown, tea.KeyTab:
		if m.platformCursor < len(m.platforms)-1 {
			m.platformCursor++
		}
	case tea.KeySpace:
		if m.platformSelected[m.platformCursor] {
			delete(m.platformSelected, m.platformCursor)
		} else {
			m.platformSelected[m.platformCursor] = true
		}
	case tea.KeyEnter:
		if len(m.platformSelected) == 0 {
			return m, nil // must select at least one
		}
		// Build ordered list of selected platforms
		m.selectedPlatforms = nil
		for i, name := range m.platforms {
			if m.platformSelected[i] {
				m.selectedPlatforms = append(m.selectedPlatforms, name)
			}
		}
		m.currentPlatIdx = 0
		m.phase = phaseTokenInput
		m.tokenInput.SetValue("")
		m.tokenInput.Focus()
		m.validationErr = ""
		return m, m.tokenInput.Cursor.BlinkCmd()
	}

	return m, nil
}

func (m WizardModel) updateTokenInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if ok && key.Type == tea.KeyEnter {
		token := strings.TrimSpace(m.tokenInput.Value())
		if token == "" {
			return m, nil
		}
		// Store token and start validation
		currentPlat := m.selectedPlatforms[m.currentPlatIdx]
		m.rawTokens[currentPlat] = token
		m.phase = phaseTokenValidate
		m.validationErr = ""
		return m, validateTokenCmd(currentPlat, token)
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.tokenInput, cmd = m.tokenInput.Update(msg)
	return m, cmd
}

func (m WizardModel) updateProjectName(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if ok && key.Type == tea.KeyEnter {
		name := strings.TrimSpace(m.projectInput.Value())
		if name == "" {
			return m, nil
		}
		m.savedProject = name

		if len(m.allServices) == 0 {
			// No services discovered — skip to saving
			m.phase = phaseSaving
			return m, saveConfigCmd(m.savedProject, m.rawTokens, nil)
		}

		// Pre-select all services
		for i := range m.allServices {
			m.serviceSelected[i] = true
		}
		m.phase = phaseServiceSelect
		return m, nil
	}

	var cmd tea.Cmd
	m.projectInput, cmd = m.projectInput.Update(msg)
	return m, cmd
}

func (m WizardModel) updateServiceSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		if m.serviceCursor > 0 {
			m.serviceCursor--
		}
	case tea.KeyDown, tea.KeyTab:
		if m.serviceCursor < len(m.allServices)-1 {
			m.serviceCursor++
		}
	case tea.KeySpace:
		if m.serviceSelected[m.serviceCursor] {
			delete(m.serviceSelected, m.serviceCursor)
		} else {
			m.serviceSelected[m.serviceCursor] = true
		}
	case tea.KeyEnter:
		// Collect selected services
		var selected []platform.DiscoveredService
		for i, svc := range m.allServices {
			if m.serviceSelected[i] {
				selected = append(selected, svc)
			}
		}
		m.phase = phaseSaving
		return m, saveConfigCmd(m.savedProject, m.rawTokens, selected)
	}

	return m, nil
}

func (m WizardModel) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

// --- Async message handlers ---

func (m WizardModel) handleTokenValidated(msg tokenValidatedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.validationErr = msg.err.Error()
		m.phase = phaseTokenInput
		m.tokenInput.SetValue("")
		m.tokenInput.Focus()
		return m, m.tokenInput.Cursor.BlinkCmd()
	}

	// Accumulate discovered services
	m.allServices = append(m.allServices, msg.services...)

	// Move to next platform or to project name
	m.currentPlatIdx++
	if m.currentPlatIdx < len(m.selectedPlatforms) {
		m.phase = phaseTokenInput
		m.tokenInput.SetValue("")
		m.tokenInput.Focus()
		m.validationErr = ""
		return m, m.tokenInput.Cursor.BlinkCmd()
	}

	// All platforms done — move to project name
	m.phase = phaseProjectName
	m.projectInput.Focus()
	return m, m.projectInput.Cursor.BlinkCmd()
}

func (m WizardModel) handleConfigSaved(msg configSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.saveErr = msg.err.Error()
	}
	m.phase = phaseDone
	return m, nil
}

// --- Async commands ---

func validateTokenCmd(name, token string) tea.Cmd {
	return func() tea.Msg {
		p, err := platform.Get(name, token)
		if err != nil {
			return tokenValidatedMsg{platform: name, err: err}
		}

		if err := p.Validate(token); err != nil {
			return tokenValidatedMsg{platform: name, err: err}
		}

		// Also discover services if supported
		var services []platform.DiscoveredService
		if disc, ok := p.(platform.Discoverer); ok {
			services, _ = disc.DiscoverServices()
		}

		return tokenValidatedMsg{platform: name, services: services}
	}
}

func saveConfigCmd(projectName string, rawTokens map[string]string, services []platform.DiscoveredService) tea.Cmd {
	return func() tea.Msg {
		key, err := config.LoadOrCreateKey()
		if err != nil {
			return configSavedMsg{err: fmt.Errorf("load key: %w", err)}
		}

		cfg, err := config.Load()
		if err != nil {
			return configSavedMsg{err: fmt.Errorf("load config: %w", err)}
		}

		// Encrypt and store tokens
		for name, token := range rawTokens {
			enc, err := config.Encrypt(key, token)
			if err != nil {
				return configSavedMsg{err: fmt.Errorf("encrypt %s token: %w", name, err)}
			}
			cfg.Platforms[name] = config.PlatformConfig{Token: enc}
		}

		// Build topology
		var topology []config.ServiceEntry
		for _, svc := range services {
			topology = append(topology, config.ServiceEntry{
				Name:     svc.Name,
				Platform: svc.Platform,
				ID:       svc.ID,
			})
		}

		cfg.Projects[projectName] = config.ProjectConfig{Topology: topology}
		cfg.DefaultProject = projectName

		if err := config.Save(cfg); err != nil {
			return configSavedMsg{err: fmt.Errorf("save config: %w", err)}
		}

		return configSavedMsg{}
	}
}

// --- View ---

var (
	wizardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPrimary).
				MarginBottom(1)

	wizardBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(ColorHealthy).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)
)

func (m WizardModel) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	switch m.phase {
	case phaseWelcome:
		s.WriteString(m.viewWelcome())
	case phasePlatformSelect:
		s.WriteString(m.viewPlatformSelect())
	case phaseTokenInput:
		s.WriteString(m.viewTokenInput())
	case phaseTokenValidate:
		s.WriteString(m.viewTokenValidate())
	case phaseProjectName:
		s.WriteString(m.viewProjectName())
	case phaseServiceSelect:
		s.WriteString(m.viewServiceSelect())
	case phaseSaving:
		s.WriteString(m.viewSaving())
	case phaseDone:
		s.WriteString(m.viewDone())
	}

	return s.String()
}

func (m WizardModel) viewWelcome() string {
	title := wizardTitleStyle.Render(IconRocket + " Welcome to Orbit")
	body := fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		title,
		"Orbit helps you monitor services across cloud platforms.\n"+
			"This wizard will walk you through connecting your platforms,\n"+
			"discovering services, and creating your first project.",
		dimStyle.Render("Press Enter to get started..."),
	)
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewPlatformSelect() string {
	title := wizardTitleStyle.Render("Select platforms to connect")
	var items strings.Builder
	for i, name := range m.platforms {
		cursor := "  "
		if i == m.platformCursor {
			cursor = cursorStyle.Render("> ")
		}
		check := "[ ] "
		if m.platformSelected[i] {
			check = selectedStyle.Render("[x] ")
		}
		label := name
		if i == m.platformCursor {
			label = cursorStyle.Render(name)
		}
		items.WriteString(fmt.Sprintf("%s%s%s\n", cursor, check, label))
	}
	help := dimStyle.Render("↑/↓ move • Space select • Enter confirm")
	body := fmt.Sprintf("%s\n\n%s\n%s", title, items.String(), help)
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewTokenInput() string {
	name := m.selectedPlatforms[m.currentPlatIdx]
	title := wizardTitleStyle.Render(fmt.Sprintf("Connect %s (%d/%d)", name, m.currentPlatIdx+1, len(m.selectedPlatforms)))

	tokenURL := platform.TokenURL(name)
	urlLine := ""
	if tokenURL != "" {
		urlLine = dimStyle.Render("Get your token at: "+tokenURL) + "\n\n"
	}

	errLine := ""
	if m.validationErr != "" {
		errLine = "\n" + ErrorStyle.Render("Error: "+m.validationErr) + "\n"
	}

	body := fmt.Sprintf(
		"%s\n\n%s%s%s\n\n%s",
		title,
		urlLine,
		"API Token: "+m.tokenInput.View(),
		errLine,
		dimStyle.Render("Enter to validate • Ctrl+C to quit"),
	)
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewTokenValidate() string {
	name := m.selectedPlatforms[m.currentPlatIdx]
	title := wizardTitleStyle.Render(fmt.Sprintf("Validating %s token...", name))
	body := fmt.Sprintf("%s\n\n%s", title, dimStyle.Render("Connecting to API and discovering services..."))
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewProjectName() string {
	title := wizardTitleStyle.Render("Name your project")

	// Show connection summary
	var summary strings.Builder
	for _, name := range m.selectedPlatforms {
		summary.WriteString(fmt.Sprintf("  %s %s\n", HealthyStyle.Render(IconHealthy), name))
	}

	svcCount := len(m.allServices)
	discovered := dimStyle.Render(fmt.Sprintf("%d services discovered across %d platforms", svcCount, len(m.selectedPlatforms)))

	body := fmt.Sprintf(
		"%s\n\n%s\n%s\n\n%s %s\n\n%s",
		title,
		summary.String(),
		discovered,
		"Project name:",
		m.projectInput.View(),
		dimStyle.Render("Enter to continue"),
	)
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewServiceSelect() string {
	title := wizardTitleStyle.Render("Select services to monitor")

	var items strings.Builder
	for i, svc := range m.allServices {
		cursor := "  "
		if i == m.serviceCursor {
			cursor = cursorStyle.Render("> ")
		}
		check := "[ ] "
		if m.serviceSelected[i] {
			check = selectedStyle.Render("[x] ")
		}
		label := fmt.Sprintf("%s %s", svc.Name, dimStyle.Render("("+svc.Platform+")"))
		if i == m.serviceCursor {
			label = fmt.Sprintf("%s %s", cursorStyle.Render(svc.Name), dimStyle.Render("("+svc.Platform+")"))
		}
		items.WriteString(fmt.Sprintf("%s%s%s\n", cursor, check, label))
	}

	help := dimStyle.Render("↑/↓ move • Space toggle • Enter confirm")
	body := fmt.Sprintf("%s\n\n%s\n%s", title, items.String(), help)
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewSaving() string {
	title := wizardTitleStyle.Render("Saving configuration...")
	body := fmt.Sprintf("%s\n\n%s", title, dimStyle.Render("Encrypting tokens and writing config..."))
	return wizardBoxStyle.Render(body)
}

func (m WizardModel) viewDone() string {
	if m.saveErr != "" {
		title := wizardTitleStyle.Render(IconError + " Setup failed")
		body := fmt.Sprintf("%s\n\n%s\n\n%s",
			title,
			ErrorStyle.Render(m.saveErr),
			dimStyle.Render("Press any key to exit"),
		)
		return wizardBoxStyle.Render(body)
	}

	title := wizardTitleStyle.Render(IconRocket + " Setup complete!")

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Project: %s\n\n", ProjectTitleStyle.Render(m.savedProject)))

	summary.WriteString("Platforms:\n")
	for _, name := range m.selectedPlatforms {
		summary.WriteString(fmt.Sprintf("  %s %s\n", HealthyStyle.Render(IconHealthy), name))
	}

	// Count selected services
	selected := 0
	for _, v := range m.serviceSelected {
		if v {
			selected++
		}
	}
	if selected > 0 {
		summary.WriteString(fmt.Sprintf("\nServices: %d monitored\n", selected))
	}

	summary.WriteString(fmt.Sprintf("\nRun %s to see your services.", HealthyStyle.Render("orbit status")))

	body := fmt.Sprintf("%s\n\n%s\n\n%s",
		title,
		summary.String(),
		dimStyle.Render("Press any key to exit"),
	)
	return wizardBoxStyle.Render(body)
}
