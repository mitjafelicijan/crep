package main

// Monolithic core of the application. Manages the global editor state, buffer
// lifecycle, UI rendering, and coordination between different components like
// LSP, Ollama, and Syntax.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/nsf/termbox-go"
)

// Mode represents the current operational state of the editor.
type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeCommand     // Colon command line mode
	ModeFuzzy       // File/buffer fuzzy finder mode
	ModeVisual      // Character-wise selection
	ModeVisualLine  // Line-wise selection
	ModeFind        // In-file search mode (/)
	ModeReplace     // Pattern replacement mode
	ModeVisualBlock // Columnar selection
	ModeConfirm     // Yes/No confirmation prompt
)

type FuzzyType int

const (
	FuzzyModeFile FuzzyType = iota
	FuzzyModeBuffer
	FuzzyModeWarning
)

type Jump struct {
	filename string
	cursorX  int
	cursorY  int
}

type DiagnosticItem struct {
	filename  string
	line      int
	character int
	message   string
	severity  int
}

// MatchRange represents a span of text matched by search or replace.
type MatchRange struct {
	startLine int
	startCol  int
	endLine   int
	endCol    int
}

// Editor is the main controller struct that holds all global state.
type Editor struct {
	buffers            []*Buffer        // All open file buffers.
	activeBufferIndex  int              // Currently visible buffer.
	mode               Mode             // Current editor mode.
	clipboard          []rune           // Basic internal clipboard.
	pendingKey         rune             // Stores the first character of a multi-key command (e.g., 'g').
	commandBuffer      []rune           // Input for the : command line.
	commandCursorX     int              // Cursor position within commandBuffer.
	commandHistory     []string         // History of executed commands.
	commandHistoryIdx  int              // Current position in command history (-1 = not navigating).
	findBuffer         []rune           // Input for the / find line.
	findSavedSearch    string           // Search term before incremental search started.
	lastSearch         string           // The last searched term (for 'n'/'N').
	fuzzyBuffer        []rune           // Filter pattern in fuzzy finder.
	fuzzyResults       []string         // Filtered items shown to the user.
	fuzzyResultIndices []int            // Map from displayed results back to original candidates.
	fuzzyIndex         int              // Highlighted item in the result list.
	fuzzyScroll        int              // Viewport offset for the result list.
	fuzzyCandidates    []string         // Raw list of all possible items (files/buffers/etc.).
	fuzzyType          FuzzyType        // What the fuzzy finder is searching for.
	fuzzyDiagnostics   []DiagnosticItem // Diagnostics from all buffers (accessible via finder).
	mouseEnabled       bool             // Toggle for mouse support.
	visualStartX       int              // Starting anchor for visual selection.
	visualStartY       int              // Starting anchor for visual selection.
	logMessages        []string         // Internal debug logs shown in the Log window.
	maxLogMessages     int              // Maximum capacity of the log ring buffer.
	showDebugLog       bool             // Visibility toggle for the log window.
	jumplist           []Jump           // History of cursor locations (for Ctrl-O/Ctrl-I).
	jumpIndex          int              // Current position in the jumplist.
	message            string           // Status message shown at the bottom.
	commands           *Command         // Command handler instance.
	devMode            bool             // Internal developer mode toggle.
	ollamaClient       *OllamaClient    // Client for local AI features.
	introDismissed     bool             // Whether the splash screen was hidden.

	// Replace mode state (regex replacement UI)
	replaceInput     []rune
	replaceSelStartX int
	replaceSelStartY int
	replaceSelEndX   int
	replaceSelEndY   int
	replaceMatches   []MatchRange
	pendingConfirm   func() // Callback for the confirmation mode.
	hoverContent     string // Text content for the LSP hover popup.
	showHover        bool   // Visibility toggle for the hover popup.

	// Autocomplete state
	showAutocomplete   bool             // Visibility toggle for the autocomplete popup.
	autocompleteItems  []CompletionItem // List of completion suggestions from LSP.
	autocompleteIndex  int              // Currently selected item in the autocomplete list.
	autocompleteScroll int              // Scroll offset for autocomplete popup.
}

// activeBuffer returns the Buffer currently being edited.
func (e *Editor) activeBuffer() *Buffer {
	if len(e.buffers) == 0 {
		return nil
	}
	return e.buffers[e.activeBufferIndex]
}

func (e *Editor) useTabs() bool {
	b := e.activeBuffer()
	if b == nil || b.fileType == nil {
		return false
	}
	return b.fileType.UseTabs
}

func (e *Editor) markModified() {
	b := e.activeBuffer()
	if b != nil {
		b.modified = true
	}
}

func (e *Editor) visualWidth(r rune, currentX int) int {
	if r == '\t' {
		b := e.activeBuffer()
		tabWidth := Config.DefaultTabWidth
		if b != nil && b.fileType != nil {
			tabWidth = b.fileType.TabWidth
		}
		return tabWidth - (currentX % tabWidth)
	}
	return 1
}

// bufferToVisual converts a buffer column index to its visual column index (explaining tabs).
func (e *Editor) bufferToVisual(line []rune, bufferX int) int {
	visualX := 0
	for i := 0; i < bufferX && i < len(line); i++ {
		visualX += e.visualWidth(line[i], visualX)
	}
	return visualX
}

func (e *Editor) bufferToString(buffer [][]rune) string {
	var result strings.Builder
	for i, line := range buffer {
		result.WriteString(string(line))
		if i < len(buffer)-1 {
			result.WriteString("\n")
		}
	}
	return result.String()
}

// NewEditor creates a new editor instance with a default empty buffer.
func NewEditor(devMode bool) *Editor {
	e := &Editor{
		buffers:           []*Buffer{},
		activeBufferIndex: 0,
		mode:              ModeNormal,
		pendingKey:        0,
		commandBuffer:     []rune{},
		commandHistory:    []string{},
		commandHistoryIdx: -1,
		findBuffer:        []rune{},
		lastSearch:        "",
		fuzzyBuffer:       []rune{},
		fuzzyResults:      []string{},
		fuzzyIndex:        0,
		fuzzyScroll:       0,
		fuzzyCandidates:   []string{},
		mouseEnabled:      true,
		logMessages:       []string{},
		maxLogMessages:    50,
		showDebugLog:      false,
		jumplist:          []Jump{},
		jumpIndex:         -1,
		devMode:           devMode,
		ollamaClient:      NewOllamaClient(),
	}
	e.addLog("Editor", "Editor initialized")
	// Add an initial empty buffer with default file type
	defaultType := fileTypes[len(fileTypes)-1]
	e.buffers = append(e.buffers, &Buffer{
		buffer:    [][]rune{{}},
		undoStack: []HistoryState{},
		redoStack: []HistoryState{},
		fileType:  defaultType,
	})
	e.commands = &Command{e: e}
	return e
}

func (e *Editor) addLog(group, msg string) {
	t := time.Now()
	timestamp := fmt.Sprintf("[%02d:%01d:%02d]", t.Hour(), t.Minute(), t.Second())
	logMsg := fmt.Sprintf("%s [%s] %s", timestamp, group, msg)
	e.logMessages = append(e.logMessages, logMsg)

	if len(e.logMessages) > e.maxLogMessages {
		e.logMessages = e.logMessages[len(e.logMessages)-e.maxLogMessages:]
	}

	if Config.UseLogFile {
		f, err := os.OpenFile(Config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			f.WriteString(logMsg + "\n")
		}
	}
}

func (e *Editor) toggleDebugWindow() {
	e.showDebugLog = !e.showDebugLog
}

// LoadFile reads a file from disk into the active buffer.
func (e *Editor) LoadFile(filename string) error {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		// Create subfolders if they don't exist
		if dir := filepath.Dir(filename); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directories: %v", err)
			}
		}
		// Create the file
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("failed to create file: %v", err)
		}
		file.Close()
		// Get info for the newly created file
		info, err = os.Stat(filename)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	err = e.LoadFromReader(filename, file)
	if err == nil {
		if info != nil {
			e.activeBuffer().lastModTime = info.ModTime()
		}
	}
	return err
}

func (e *Editor) LoadFromReader(filename string, r io.Reader) error {
	ft := getFileType(filename)

	var bufferLines [][]rune
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF && line == "" {
			break
		}

		// Remove trailing newline
		trimmedLine := strings.TrimSuffix(line, "\n")
		trimmedLine = strings.TrimSuffix(trimmedLine, "\r")

		if !ft.UseTabs {
			trimmedLine = strings.ReplaceAll(trimmedLine, "\t", strings.Repeat(" ", ft.TabWidth))
		}
		bufferLines = append(bufferLines, []rune(trimmedLine))

		if err == io.EOF {
			break
		}
	}

	// Ensure buffer is never empty
	if len(bufferLines) == 0 {
		bufferLines = [][]rune{{}}
	}

	// Check if we should update current buffer or add a new one
	b := e.activeBuffer()
	if b != nil && b.filename == "" && len(b.buffer) == 1 && len(b.buffer[0]) == 0 {
		// reuse current empty buffer
		b.filename = filename
		b.buffer = bufferLines
		b.PrimaryCursor().X = 0
		b.PrimaryCursor().Y = 0
		b.scrollX = 0
		b.scrollY = 0
		b.undoStack = []HistoryState{}
		b.redoStack = []HistoryState{}
		b.redoStack = []HistoryState{}
		b.fileType = ft

		// Initialize Syntax Highlighter
		syntax := NewSyntaxHighlighter(ft.Name, e.addLog)
		if syntax != nil {
			content := e.bufferToString(bufferLines)
			syntax.Parse([]byte(content))
			b.syntax = syntax
		}

		// Initialize LSP if enabled for this file type
		if ft.EnableLSP && ft.LSPCommand != "" {
			e.addLog("LSP", fmt.Sprintf("Starting LSP for %s", filepath.Base(filename)))
			content := e.bufferToString(bufferLines)
			lspClient, err := NewLSPClient(filename, content, e.addLog, ft)
			if err == nil {
				b.lspClient = lspClient
				e.addLog("LSP", "LSP client initialized successfully")
			} else {
				e.addLog("LSP", fmt.Sprintf("LSP init failed: %v", err))
			}
		}
	} else {
		// add new buffer
		newB := &Buffer{
			buffer:    bufferLines,
			filename:  filename,
			undoStack: []HistoryState{},
			redoStack: []HistoryState{},
			fileType:  ft,
		}

		// Initialize Syntax Highlighter
		syntax := NewSyntaxHighlighter(ft.Name, e.addLog)
		if syntax != nil {
			content := e.bufferToString(bufferLines)
			syntax.Parse([]byte(content))
			newB.syntax = syntax
		}

		// Initialize LSP if enabled for this file type
		if ft.EnableLSP && ft.LSPCommand != "" {
			e.addLog("LSP", fmt.Sprintf("Starting LSP for %s", filepath.Base(filename)))
			content := e.bufferToString(bufferLines)
			lspClient, err := NewLSPClient(filename, content, e.addLog, ft)
			if err == nil {
				newB.lspClient = lspClient
				e.addLog("LSP", "LSP client initialized successfully")
			} else {
				e.addLog("LSP", fmt.Sprintf("LSP init failed: %v", err))
			}
		}

		e.buffers = append(e.buffers, newB)
		e.activeBufferIndex = len(e.buffers) - 1
	}

	return nil
}

// SaveFile writes the active buffer content back to disk.
func (e *Editor) SaveFile(force bool) error {
	b := e.activeBuffer()
	if b == nil || b.filename == "" {
		return fmt.Errorf("no filename")
	}

	// Check for external modifications unless forced.
	if !force {
		info, err := os.Stat(b.filename)
		if err == nil && info.ModTime().After(b.lastModTime) {
			return fmt.Errorf("file changed on disk")
		}
	}

	file, err := os.Create(b.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for i, line := range b.buffer {
		_, err := writer.WriteString(string(line))
		if err != nil {
			return err
		}
		// Write newline if not the last line (or if buffer should end with newline).
		if i < len(b.buffer)-1 || (len(b.buffer) > 0 && (len(b.buffer) > 1 || len(b.buffer[0]) > 0)) {
			_, err = writer.WriteString("\n")
			if err != nil {
				return err
			}
		}
	}
	err = writer.Flush()
	if err == nil {
		b.modified = false
		info, err := os.Stat(b.filename)
		if err == nil {
			b.lastModTime = info.ModTime()
		}
	}
	return err
}

func (e *Editor) nextBuffer() {
	if len(e.buffers) > 0 {
		e.activeBufferIndex = (e.activeBufferIndex + 1) % len(e.buffers)
	}
}

func (e *Editor) prevBuffer() {
	if len(e.buffers) > 0 {
		e.activeBufferIndex = (e.activeBufferIndex - 1 + len(e.buffers)) % len(e.buffers)
	}
}

func (e *Editor) ReloadBuffer(b *Buffer) error {
	if b == nil || b.filename == "" {
		return fmt.Errorf("no filename")
	}

	info, err := os.Stat(b.filename)
	if err != nil {
		return err
	}

	file, err := os.Open(b.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	ft := getFileType(b.filename)

	var bufferLines [][]rune
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF && line == "" {
			break
		}

		trimmedLine := strings.TrimSuffix(line, "\n")
		trimmedLine = strings.TrimSuffix(trimmedLine, "\r")

		if !ft.UseTabs {
			trimmedLine = strings.ReplaceAll(trimmedLine, "\t", strings.Repeat(" ", ft.TabWidth))
		}
		bufferLines = append(bufferLines, []rune(trimmedLine))

		if err == io.EOF {
			break
		}
	}

	if len(bufferLines) == 0 {
		bufferLines = [][]rune{{}}
	}

	b.buffer = bufferLines
	b.lastModTime = info.ModTime()
	b.modified = false

	// Adjust cursors if they are out of bounds
	for i := range b.cursors {
		c := &b.cursors[i]
		if c.Y >= len(b.buffer) {
			c.Y = len(b.buffer) - 1
		}
		if c.Y < 0 {
			c.Y = 0
		}
		if c.X > len(b.buffer[c.Y]) {
			c.X = len(b.buffer[c.Y])
		}
	}

	// Reinitialize Syntax Highlighter
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	} else {
		syntax := NewSyntaxHighlighter(ft.Name, e.addLog)
		if syntax != nil {
			syntax.Parse([]byte(b.toString()))
			b.syntax = syntax
		}
	}

	// Update LSP if active
	if b.lspClient != nil {
		b.lspClient.SendDidChange(b.toString())
	}

	return nil
}

func (e *Editor) CheckFilesOnDisk() {
	for _, b := range e.buffers {
		if b.filename == "" {
			continue
		}

		info, err := os.Stat(b.filename)
		if err != nil {
			continue
		}

		if info.ModTime().After(b.lastModTime) {
			isActive := b == e.activeBuffer()
			if !b.modified {
				// Auto reload if not dirty
				err := e.ReloadBuffer(b)
				if err == nil {
					e.addLog("Editor", fmt.Sprintf("Auto-reloaded \"%s\" (changed on disk)", filepath.Base(b.filename)))
					if isActive {
						e.message = fmt.Sprintf("\"%s\" reloaded from disk", filepath.Base(b.filename))
					}
				} else {
					e.addLog("Editor", fmt.Sprintf("Failed to auto-reload \"%s\": %v", b.filename, err))
				}
			} else if isActive {
				// Buffer is dirty, just notify the user (only if active)
				e.message = fmt.Sprintf("WARNING: \"%s\" changed on disk. Use :reload to update.", filepath.Base(b.filename))
				e.addLog("Editor", fmt.Sprintf("\"%s\" changed on disk but buffer is modified", b.filename))
				// Update lastModTime so we don't spam the message?
				// Actually, better to keep it so they realize it's still different.
				// But we should probably only message if it's the active buffer.
			}
		}
	}
}

func (e *Editor) PeriodicFileChangesCheck() {
	go func() {
		for {
			time.Sleep(Config.FileCheckInterval)
			termbox.Interrupt()
		}
	}()
}

func (e *Editor) startFileFuzzyFinder() {
	e.fuzzyCandidates = []string{}
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		e.fuzzyCandidates = append(e.fuzzyCandidates, path)
		return nil
	})
	e.fuzzyBuffer = []rune{}
	e.fuzzyIndex = 0
	e.fuzzyType = FuzzyModeFile
	e.updateFuzzyResults()
	e.mode = ModeFuzzy
}

func (e *Editor) startBufferFuzzyFinder() {
	e.fuzzyCandidates = []string{}
	for _, b := range e.buffers {
		name := b.filename
		if name == "" {
			name = "[No Name]"
		}
		e.fuzzyCandidates = append(e.fuzzyCandidates, name)
	}
	e.fuzzyBuffer = []rune{}
	e.fuzzyIndex = 0
	e.fuzzyType = FuzzyModeBuffer
	e.updateFuzzyResults()
	e.mode = ModeFuzzy
}

func (e *Editor) startWarningsFuzzyFinder() {
	e.fuzzyCandidates = []string{}
	e.fuzzyDiagnostics = []DiagnosticItem{}

	// Collect diagnostics from all buffers
	for _, b := range e.buffers {
		if len(b.diagnostics) == 0 {
			continue
		}

		filename := b.filename
		if filename == "" {
			filename = "[No Name]"
		} else {
			filename = filepath.Base(filename)
		}

		for _, diag := range b.diagnostics {
			// Format: [E] filename:line message
			severityStr := "?"
			switch diag.Severity {
			case 1:
				severityStr = "E"
			case 2:
				severityStr = "W"
			case 3:
				severityStr = "I"
			case 4:
				severityStr = "H"
			}

			formattedDiag := fmt.Sprintf("[%s] %s:%d %s",
				severityStr,
				filename,
				diag.Range.Start.Line+1, // Convert to 1-indexed
				diag.Message)

			e.fuzzyCandidates = append(e.fuzzyCandidates, formattedDiag)
			e.fuzzyDiagnostics = append(e.fuzzyDiagnostics, DiagnosticItem{
				filename:  b.filename,
				line:      diag.Range.Start.Line,
				character: diag.Range.Start.Character,
				message:   diag.Message,
				severity:  diag.Severity,
			})
		}
	}

	e.fuzzyBuffer = []rune{}
	e.fuzzyIndex = 0
	e.fuzzyType = FuzzyModeWarning
	e.updateFuzzyResults()
	e.mode = ModeFuzzy
}

func fuzzyMatch(query, target string) (int, bool) {
	if query == "" {
		return 0, true
	}

	query = strings.ToLower(query)
	targetLower := strings.ToLower(target)

	score := 0
	targetIdx := 0
	lastMatchIdx := -1

	for _, qRune := range query {
		found := false
		for i := targetIdx; i < len(targetLower); i++ {
			if rune(targetLower[i]) == qRune {
				// Bonus for consecutive matches
				if lastMatchIdx != -1 && i == lastMatchIdx+1 {
					score += 10
				}

				// Bonus for matches after separators
				if i == 0 || targetLower[i-1] == '/' || targetLower[i-1] == '_' || targetLower[i-1] == '.' || targetLower[i-1] == '-' {
					score += 20
				}

				// Penalty for gaps
				if lastMatchIdx != -1 {
					score -= (i - lastMatchIdx - 1)
				}

				score += 5 // Base match score
				lastMatchIdx = i
				targetIdx = i + 1
				found = true
				break
			}
		}
		if !found {
			return 0, false
		}
	}

	// Substring match bonus
	if strings.Contains(targetLower, query) {
		score += 50
	}

	// Exact match bonus
	if targetLower == query {
		score += 100
	}

	return score, true
}

func (e *Editor) updateFuzzyResults() {
	query := string(e.fuzzyBuffer)
	if query == "" {
		e.fuzzyResults = make([]string, len(e.fuzzyCandidates))
		e.fuzzyResultIndices = make([]int, len(e.fuzzyCandidates))
		copy(e.fuzzyResults, e.fuzzyCandidates)
		for i := range e.fuzzyResultIndices {
			e.fuzzyResultIndices[i] = i
		}
	} else {
		type result struct {
			path  string
			index int
			score int
		}
		var results []result
		for i, candidate := range e.fuzzyCandidates {
			if score, ok := fuzzyMatch(query, candidate); ok {
				results = append(results, result{candidate, i, score})
			}
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].score > results[j].score
		})

		e.fuzzyResults = make([]string, len(results))
		e.fuzzyResultIndices = make([]int, len(results))
		for i, res := range results {
			e.fuzzyResults[i] = res.path
			e.fuzzyResultIndices[i] = res.index
		}
	}
	if e.fuzzyIndex >= len(e.fuzzyResults) {
		e.fuzzyIndex = 0
	}
	e.fuzzyScroll = 0
}

func (e *Editor) openSelectedFile() {
	if len(e.fuzzyResults) == 0 {
		return
	}
	selection := e.fuzzyResults[e.fuzzyIndex]

	if e.fuzzyType == FuzzyModeFile {
		err := e.LoadFile(selection)
		if err == nil {
			e.mode = ModeNormal
		}
	} else if e.fuzzyType == FuzzyModeBuffer {
		for i, b := range e.buffers {
			name := b.filename
			if name == "" {
				name = "[No Name]"
			}
			if name == selection {
				e.activeBufferIndex = i
				e.mode = ModeNormal
				break
			}
		}
	} else if e.fuzzyType == FuzzyModeWarning {
		if e.fuzzyIndex >= len(e.fuzzyResults) || e.fuzzyIndex >= len(e.fuzzyResultIndices) {
			return
		}

		// Get the original candidate index
		diagIndex := e.fuzzyResultIndices[e.fuzzyIndex]

		if diagIndex < 0 || diagIndex >= len(e.fuzzyDiagnostics) {
			return
		}

		diagItem := e.fuzzyDiagnostics[diagIndex]

		// Find or load the buffer with this file
		bufferIndex := -1
		for i, b := range e.buffers {
			if b.filename == diagItem.filename {
				bufferIndex = i
				break
			}
		}

		// If buffer not found, try to load it
		if bufferIndex == -1 && diagItem.filename != "" {
			err := e.LoadFile(diagItem.filename)
			if err == nil {
				bufferIndex = e.activeBufferIndex
			}
		}

		// Navigate to the diagnostic location
		if bufferIndex != -1 {
			e.activeBufferIndex = bufferIndex
			b := e.activeBuffer()
			if b != nil {
				// Set cursor to diagnostic line and character
				if diagItem.line < len(b.buffer) {
					b.PrimaryCursor().Y = diagItem.line
					if diagItem.character < len(b.buffer[diagItem.line]) {
						b.PrimaryCursor().X = diagItem.character
					} else {
						b.PrimaryCursor().X = 0
					}
				}
				// Center the screen on the diagnostic line
				e.centerScreen()
			}
			e.mode = ModeNormal
		}
	}
}

func (e *Editor) fuzzyMove(dir int) {
	if len(e.fuzzyResults) == 0 {
		return
	}
	e.fuzzyIndex += dir
	if e.fuzzyIndex < 0 {
		e.fuzzyIndex = len(e.fuzzyResults) - 1
	} else if e.fuzzyIndex >= len(e.fuzzyResults) {
		e.fuzzyIndex = 0
	}

	// Adjust scroll
	if e.fuzzyIndex < e.fuzzyScroll {
		e.fuzzyScroll = e.fuzzyIndex
	} else if e.fuzzyIndex >= e.fuzzyScroll+Config.FuzzyFinderHeight {
		e.fuzzyScroll = e.fuzzyIndex - Config.FuzzyFinderHeight + 1
	}

	// Special case for wrapping
	if e.fuzzyIndex == len(e.fuzzyResults)-1 && e.fuzzyScroll == 0 && len(e.fuzzyResults) > Config.FuzzyFinderHeight {
		e.fuzzyScroll = len(e.fuzzyResults) - Config.FuzzyFinderHeight
	}
	if e.fuzzyIndex == 0 && e.fuzzyScroll > 0 {
		e.fuzzyScroll = 0
	}
}

// insertTab inserts either a literal tab character or an equivalent number of spaces.
func (e *Editor) insertTab() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if e.useTabs() {
		e.insertRune('\t')
	} else {
		tabWidth := Config.DefaultTabWidth
		if b.fileType != nil {
			tabWidth = b.fileType.TabWidth
		}
		for i := 0; i < tabWidth; i++ {
			e.insertRune(' ')
		}
	}
}

// getSortedCursorsDesc returns a list of cursor pointers sorted by position (bottom-to-top, right-to-left).
// This sorting is CRITICAL for concurrent text edits to avoid offset corruption.
func (e *Editor) getSortedCursorsDesc() []*Cursor {
	b := e.activeBuffer()
	if b == nil {
		return nil
	}
	cursors := make([]*Cursor, len(b.cursors))
	for i := range b.cursors {
		cursors[i] = &b.cursors[i]
	}
	sort.Slice(cursors, func(i, j int) bool {
		if cursors[i].Y != cursors[j].Y {
			return cursors[i].Y > cursors[j].Y // Lower rows first.
		}
		return cursors[i].X > cursors[j].X // Later characters in row first.
	})
	return cursors
}

func (e *Editor) insertRune(r rune) {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursors := e.getSortedCursorsDesc()
	for _, c := range cursors {
		line := b.buffer[c.Y]
		newLine := make([]rune, len(line)+1)
		copy(newLine[:c.X], line[:c.X])
		newLine[c.X] = r
		copy(newLine[c.X+1:], line[c.X:])
		b.buffer[c.Y] = newLine
		c.X++

		// Handle syntax update
		if b.syntax != nil {
			insertedBytes := uint32(len(string(r)))
			b.handleEdit(c.Y, c.X-1, 0, insertedBytes, c.Y, b.getLineByteOffset(line, c.X-1), c.Y, b.getLineByteOffset(newLine, c.X))
		}
	}

	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()

	// Notify LSP of the change
	if b.lspClient != nil {
		b.lspClient.SendDidChange(b.toString())
	}
}

// DeleteChar removes the character directly under the cursor.
func (e *Editor) DeleteChar() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursors := e.getSortedCursorsDesc()
	for _, c := range cursors {
		if c.Y >= len(b.buffer) || c.X >= len(b.buffer[c.Y]) {
			continue
		}

		line := b.buffer[c.Y]
		// Store deleted character in clipboard (primary cursor only).
		if c == b.PrimaryCursor() {
			e.clipboard = []rune{line[c.X]}
		}

		deletedBytes := uint32(len(string(line[c.X])))
		newLine := append(line[:c.X], line[c.X+1:]...)
		b.buffer[c.Y] = newLine

		// Ensure cursor doesn't drift past the new end of line.
		if c.X > 0 && c.X >= len(newLine) {
			c.X = len(newLine) - 1
			if c.X < 0 {
				c.X = 0
			}
		}

		if b.syntax != nil {
			oldColBytes := b.getLineByteOffset(line, c.X)
			newColBytes := b.getLineByteOffset(newLine, c.X)
			b.handleEdit(c.Y, c.X, deletedBytes, 0, c.Y, oldColBytes+deletedBytes, c.Y, newColBytes)
		}
	}
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) backspace() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursors := e.getSortedCursorsDesc()
	for _, c := range cursors {
		if c.X > 0 {
			line := b.buffer[c.Y]
			deletedChar := line[c.X-1]
			newLine := append(line[:c.X-1], line[c.X:]...)
			b.buffer[c.Y] = newLine
			c.X--

			if b.syntax != nil {
				deletedBytes := uint32(len(string(deletedChar)))
				oldColBytes := b.getLineByteOffset(line, c.X+1)
				newColBytes := b.getLineByteOffset(newLine, c.X)

				b.handleEdit(c.Y, c.X, deletedBytes, 0, c.Y, oldColBytes, c.Y, newColBytes)
			}
		} else if c.Y > 0 {
			// Merge with previous line
			prevLine := b.buffer[c.Y-1]
			c.X = len(prevLine)
			b.buffer[c.Y-1] = append(prevLine, b.buffer[c.Y]...)
			b.buffer = append(b.buffer[:c.Y], b.buffer[c.Y+1:]...)
			// We need to shift cursors that are 'below' the current merge point.
			for j := range b.cursors {
				if b.cursors[j].Y > c.Y {
					b.cursors[j].Y--
				}
			}

			c.Y--

			// So I need to find other cursors on the same line that haven't been processed?
			// Or just all cursors on the same line.
			for j := range b.cursors {
				if &b.cursors[j] != c && b.cursors[j].Y == c.Y+1 { // c.Y was decremented
					// This cursor was on the line we just merged
					b.cursors[j].Y--
					b.cursors[j].X += len(prevLine)
				}
			}

			if b.syntax != nil {
				b.handleEdit(c.Y, c.X, 1, 0, c.Y+1, 0, c.Y, b.getLineByteOffset(b.buffer[c.Y], c.X))
			}
		}
	}
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) getIndentation(line []rune) []rune {
	var indent []rune
	for _, r := range line {
		if r == ' ' || r == '\t' {
			indent = append(indent, r)
		} else {
			break
		}
	}
	return indent
}

// insertNewline breaks the line at cursor and handles auto-indentation.
func (e *Editor) insertNewline() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursors := e.getSortedCursorsDesc()
	for _, c := range cursors {
		line := b.buffer[c.Y]

		// Inherit indentation from the current line.
		indent := e.getIndentation(line[:c.X])

		// Auto-indent after opening braces.
		if c.X > 0 && line[c.X-1] == '{' {
			if e.useTabs() {
				indent = append(indent, '\t')
			} else {
				tabWidth := Config.DefaultTabWidth
				if b.fileType != nil {
					tabWidth = b.fileType.TabWidth
				}
				indent = append(indent, []rune(strings.Repeat(" ", tabWidth))...)
			}
		}

		remaining := make([]rune, len(line)-c.X)
		copy(remaining, line[c.X:])

		newLine := append(indent, remaining...)
		b.buffer[c.Y] = line[:c.X]

		// Insert the new line into the buffer.
		newBuffer := make([][]rune, len(b.buffer)+1)
		copy(newBuffer[:c.Y+1], b.buffer[:c.Y+1])
		newBuffer[c.Y+1] = newLine
		copy(newBuffer[c.Y+2:], b.buffer[c.Y+1:])
		b.buffer = newBuffer

		// Shift all cursors below this point, or later on this same line.
		for j := range b.cursors {
			if b.cursors[j].Y > c.Y {
				b.cursors[j].Y++
			} else if b.cursors[j].Y == c.Y && b.cursors[j].X >= c.X && &b.cursors[j] != c {
				b.cursors[j].Y++
				b.cursors[j].X = len(indent) + (b.cursors[j].X - c.X)
			}
		}

		oldCursorX := c.X
		c.Y++
		c.X = len(indent)

		if b.syntax != nil {
			insertedBytes := uint32(1 + len(string(indent)))
			b.handleEdit(c.Y-1, oldCursorX, 0, insertedBytes, c.Y-1, b.getLineByteOffset(b.buffer[c.Y-1], oldCursorX), c.Y, b.getLineByteOffset(b.buffer[c.Y], c.X))
		}
	}
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) insertLineBelow() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}
	line := b.buffer[b.PrimaryCursor().Y]
	indent := e.getIndentation(line)

	// Check if the current line ends with '{' to increase indent
	trimmedLine := strings.TrimRight(string(line), " ")
	if len(trimmedLine) > 0 && trimmedLine[len(trimmedLine)-1] == '{' {
		if e.useTabs() {
			indent = append(indent, '\t')
		} else {
			tabWidth := Config.DefaultTabWidth
			if b.fileType != nil {
				tabWidth = b.fileType.TabWidth
			}
			indent = append(indent, []rune(strings.Repeat(" ", tabWidth))...)
		}
	}

	newBuffer := make([][]rune, len(b.buffer)+1)
	copy(newBuffer[:b.PrimaryCursor().Y+1], b.buffer[:b.PrimaryCursor().Y+1])
	newBuffer[b.PrimaryCursor().Y+1] = indent
	copy(newBuffer[b.PrimaryCursor().Y+2:], b.buffer[b.PrimaryCursor().Y+1:])
	b.buffer = newBuffer

	b.PrimaryCursor().Y++
	b.PrimaryCursor().X = len(indent)

	if b.syntax != nil {
		insertedBytes := uint32(1 + len(string(indent)))
		oldLineLen := b.getLineByteOffset(line, len(line))
		b.handleEdit(b.PrimaryCursor().Y-1, len(line), 0, insertedBytes, b.PrimaryCursor().Y-1, oldLineLen, b.PrimaryCursor().Y, b.getLineByteOffset(b.buffer[b.PrimaryCursor().Y], b.PrimaryCursor().X))
	}

	e.mode = ModeInsert
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) insertLineAbove() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}
	line := b.buffer[b.PrimaryCursor().Y]
	indent := e.getIndentation(line)

	newBuffer := make([][]rune, len(b.buffer)+1)
	copy(newBuffer[:b.PrimaryCursor().Y], b.buffer[:b.PrimaryCursor().Y])
	newBuffer[b.PrimaryCursor().Y] = indent
	copy(newBuffer[b.PrimaryCursor().Y+1:], b.buffer[b.PrimaryCursor().Y:])
	b.buffer = newBuffer

	b.PrimaryCursor().X = len(indent)

	if b.syntax != nil {
		insertedBytes := uint32(1 + len(string(indent)))
		b.handleEdit(b.PrimaryCursor().Y, 0, 0, insertedBytes, b.PrimaryCursor().Y, 0, b.PrimaryCursor().Y+1, 0)
	}

	e.mode = ModeInsert
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) moveCursor(dx int, dy int) {
	b := e.activeBuffer()
	if b == nil {
		return
	}

	for i := range b.cursors {
		c := &b.cursors[i]
		if dy != 0 {
			newY := c.Y + dy
			if newY >= 0 && newY < len(b.buffer) {
				c.Y = newY
				// Snap cursorX to the end of the new line if it's currently further
				// Or restore to preferred column if moving vertically
				if c.PreferredCol > len(b.buffer[c.Y]) {
					c.X = len(b.buffer[c.Y])
				} else {
					c.X = c.PreferredCol
				}
			}
		}

		if dx != 0 {
			newX := c.X + dx
			if newX < 0 {
				if c.Y > 0 {
					c.Y--
					c.X = len(b.buffer[c.Y])
				}
			} else if newX > len(b.buffer[c.Y]) {
				if c.Y < len(b.buffer)-1 {
					c.Y++
					c.X = 0
				}
			} else {
				c.X = newX
			}
			// Update preferred column when moving horizontally
			c.PreferredCol = c.X
		}
	}
	// TODO: Merge overlapping cursors
}

func (e *Editor) mergeCursors() {
	b := e.activeBuffer()
	if b == nil || len(b.cursors) <= 1 {
		return
	}

	// Sort cursors by Y, then X
	sort.Slice(b.cursors, func(i, j int) bool {
		if b.cursors[i].Y != b.cursors[j].Y {
			return b.cursors[i].Y < b.cursors[j].Y
		}
		return b.cursors[i].X < b.cursors[j].X
	})

	// Remove duplicates
	uniqueCursors := []Cursor{b.cursors[0]}
	for i := 1; i < len(b.cursors); i++ {
		current := b.cursors[i]
		last := uniqueCursors[len(uniqueCursors)-1]

		if current.Y == last.Y && current.X == last.X {
			continue
		}
		uniqueCursors = append(uniqueCursors, current)
	}
	b.cursors = uniqueCursors
}

func (e *Editor) isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func (e *Editor) getWordUnderCursor() string {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return ""
	}
	line := b.buffer[b.PrimaryCursor().Y]
	if len(line) == 0 || b.PrimaryCursor().X >= len(line) {
		return ""
	}

	if !e.isWordChar(line[b.PrimaryCursor().X]) {
		return ""
	}

	start := b.PrimaryCursor().X
	for start > 0 && e.isWordChar(line[start-1]) {
		start--
	}

	end := b.PrimaryCursor().X
	for end < len(line) && e.isWordChar(line[end]) {
		end++
	}

	return string(line[start:end])
}

func (e *Editor) isPathChar(r rune) bool {
	return e.isWordChar(r) || r == '/' || r == '.' || r == '-' || r == '_' || r == '~' || r == '\\' || r == ':'
}

func (e *Editor) getPathUnderCursor() string {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return ""
	}
	line := b.buffer[b.PrimaryCursor().Y]
	if len(line) == 0 || b.PrimaryCursor().X >= len(line) {
		return ""
	}

	if !e.isPathChar(line[b.PrimaryCursor().X]) {
		return ""
	}

	// Start searching from the current cursor position
	start := b.PrimaryCursor().X
	for start > 0 && e.isPathChar(line[start-1]) {
		start--
	}

	end := b.PrimaryCursor().X
	for end < len(line) && e.isPathChar(line[end]) {
		end++
	}

	return string(line[start:end])
}

func (e *Editor) gotoFile() {
	path := e.getPathUnderCursor()
	if path == "" {
		e.message = "No path under cursor"
		return
	}

	// Check if the path is a URL
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		e.openURL(path)
		return
	}

	b := e.activeBuffer()
	if b == nil {
		return
	}

	// Try relative to current file
	dir := filepath.Dir(b.filename)
	targetPath := filepath.Join(dir, path)

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		// Try relative to CWD
		targetPath = path
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			e.message = "File not found: " + path
			return
		}
	}

	// Resolve absolute path for comparison
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		e.message = "Error resolving path: " + err.Error()
		return
	}

	// Check if already open
	for i, buf := range e.buffers {
		bufAbs, _ := filepath.Abs(buf.filename)
		if absPath == bufAbs {
			e.pushJump()
			e.activeBufferIndex = i
			return
		}
	}

	// Open new file
	e.pushJump()
	if err := e.LoadFile(targetPath); err != nil {
		e.message = "Error opening file: " + err.Error()
	}
}

func (e *Editor) openURL(url string) {
	var cmd string
	var args []string

	// Detect OS and set appropriate command
	switch runtime.GOOS {
	case "darwin":
		// macOS
		cmd = "open"
		args = []string{url}
	default:
		// Linux and others
		cmd = "xdg-open"
		args = []string{url}
	}

	// Execute the command
	exec := exec.Command(cmd, args...)
	if err := exec.Start(); err != nil {
		e.message = "Error opening URL: " + err.Error()
	} else {
		e.message = "Opening URL in browser..."
	}
}

// centerCursor scrolls the viewport so the cursor is in the middle of the screen.
func (e *Editor) centerCursor() {
	b := e.activeBuffer()
	if b == nil {
		return
	}

	_, h := termbox.Size()
	visibleHeight := h - 2 // Status bar and message line.
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	targetScrollY := b.PrimaryCursor().Y - (visibleHeight / 2)
	if targetScrollY < 0 {
		targetScrollY = 0
	}

	// Clamp to legitimate buffer range.
	if targetScrollY > len(b.buffer)-visibleHeight {
		targetScrollY = len(b.buffer) - visibleHeight
	}
	if targetScrollY < 0 {
		targetScrollY = 0
	}

	b.scrollY = targetScrollY
}

func (e *Editor) gotoDefinition() {
	b := e.activeBuffer()
	if b == nil || b.lspClient == nil {
		return
	}

	e.pushJump()

	locs, err := b.lspClient.Definition(b.PrimaryCursor().Y, b.PrimaryCursor().X)
	if err != nil {
		e.addLog("Editor", fmt.Sprintf("gotoDefinition error: %v", err))
		return
	}

	if len(locs) == 0 {
		e.addLog("Editor", "gotoDefinition: No definition found")
		return
	}

	loc := locs[0]
	targetPath := strings.TrimPrefix(loc.URI, "file://")

	// Find if buffer is already open
	found := false
	for i, buf := range e.buffers {
		absT, _ := filepath.Abs(targetPath)
		absB, _ := filepath.Abs(buf.filename)
		if absT == absB {
			e.activeBufferIndex = i
			found = true
			break
		}
	}

	if !found {
		if err := e.LoadFile(targetPath); err != nil {
			e.addLog("Editor", fmt.Sprintf("gotoDefinition: Failed to load %s: %v", targetPath, err))
			return
		}
	}

	b = e.activeBuffer()
	b.PrimaryCursor().Y = loc.Range.Start.Line
	b.PrimaryCursor().X = loc.Range.Start.Character

	// Ensure cursor is within bounds
	if b.PrimaryCursor().Y < 0 {
		b.PrimaryCursor().Y = 0
	}
	if b.PrimaryCursor().Y >= len(b.buffer) {
		b.PrimaryCursor().Y = len(b.buffer) - 1
	}
	if b.PrimaryCursor().X < 0 {
		b.PrimaryCursor().X = 0
	}
	if b.PrimaryCursor().X > len(b.buffer[b.PrimaryCursor().Y]) {
		b.PrimaryCursor().X = len(b.buffer[b.PrimaryCursor().Y])
	}
	e.centerCursor()
}

func (e *Editor) pushJump() {
	b := e.activeBuffer()
	if b == nil {
		return
	}

	jump := Jump{
		filename: b.filename,
		cursorX:  b.PrimaryCursor().X,
		cursorY:  b.PrimaryCursor().Y,
	}

	// If we're not at the end of the jumplist, truncate it
	if e.jumpIndex < len(e.jumplist)-1 {
		e.jumplist = e.jumplist[:e.jumpIndex+1]
	}

	// Don't push if the last jump is the same position
	if len(e.jumplist) > 0 {
		last := e.jumplist[len(e.jumplist)-1]
		if last.filename == jump.filename && last.cursorX == jump.cursorX && last.cursorY == jump.cursorY {
			return
		}
	}

	e.jumplist = append(e.jumplist, jump)
	if len(e.jumplist) > 100 {
		e.jumplist = e.jumplist[1:]
	}
	e.jumpIndex = len(e.jumplist) - 1
}

func (e *Editor) jumpBack() {
	if e.jumpIndex < 0 {
		return
	}

	// If we are at the latest jump, push the CURRENT position so we can return to it
	if e.jumpIndex == len(e.jumplist)-1 {
		b := e.activeBuffer()
		if b != nil {
			curr := Jump{filename: b.filename, cursorX: b.PrimaryCursor().X, cursorY: b.PrimaryCursor().Y}
			last := e.jumplist[e.jumpIndex]
			if curr != last {
				e.jumplist = append(e.jumplist, curr)
				e.jumpIndex = len(e.jumplist) - 2 // Point to the one before the one we just added
			} else {
				e.jumpIndex--
			}
		} else {
			e.jumpIndex--
		}
	} else {
		e.jumpIndex--
	}

	if e.jumpIndex < 0 {
		return
	}

	e.performJump(e.jumplist[e.jumpIndex])
}

func (e *Editor) jumpForward() {
	if e.jumpIndex >= len(e.jumplist)-1 {
		return
	}

	e.jumpIndex++
	e.performJump(e.jumplist[e.jumpIndex])
}

func (e *Editor) performJump(jump Jump) {
	// Find if buffer is already open
	found := false
	for i, buf := range e.buffers {
		absT, _ := filepath.Abs(jump.filename)
		absB, _ := filepath.Abs(buf.filename)
		if absT == absB {
			e.activeBufferIndex = i
			found = true
			break
		}
	}

	if !found {
		if err := e.LoadFile(jump.filename); err != nil {
			e.addLog("Editor", fmt.Sprintf("performJump: Failed to load %s: %v", jump.filename, err))
			return
		}
	}

	b := e.activeBuffer()
	b.PrimaryCursor().Y = jump.cursorY
	b.PrimaryCursor().X = jump.cursorX

	// Ensure cursor is within bounds
	if b.PrimaryCursor().Y < 0 {
		b.PrimaryCursor().Y = 0
	}
	if b.PrimaryCursor().Y >= len(b.buffer) {
		b.PrimaryCursor().Y = len(b.buffer) - 1
	}
	if b.PrimaryCursor().X < 0 {
		b.PrimaryCursor().X = 0
	}
	if b.PrimaryCursor().X > len(b.buffer[b.PrimaryCursor().Y]) {
		b.PrimaryCursor().X = len(b.buffer[b.PrimaryCursor().Y])
	}
}

// deleteWord removes a word-clump from the current cursor position.
func (e *Editor) deleteWord(includeSpaces bool) {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursors := e.getSortedCursorsDesc()
	for _, c := range cursors {
		if c.Y >= len(b.buffer) {
			continue
		}
		line := b.buffer[c.Y]
		if len(line) == 0 || c.X >= len(line) {
			continue
		}

		start := c.X
		end := start

		// Determine the boundary of the deletion based on character type.
		if e.isWordChar(line[end]) {
			// On a word character: skip word characters, then skip trailing spaces
			for end < len(line) && e.isWordChar(line[end]) {
				end++
			}
			if includeSpaces {
				for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
					end++
				}
			}
		} else if line[end] == ' ' || line[end] == '\t' {
			// On whitespace: skip all leading whitespace
			for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
				end++
			}
		} else {
			// On punctuation/other: skip those, then skip trailing spaces
			for end < len(line) && !e.isWordChar(line[end]) && line[end] != ' ' && line[end] != '\t' {
				end++
			}
			if includeSpaces {
				for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
					end++
				}
			}
		}

		// Copy to clipboard (only for primary cursor)
		if c == b.PrimaryCursor() {
			e.clipboard = make([]rune, end-start)
			copy(e.clipboard, line[start:end])
		}

		// Delete from start to end
		newLine := append(line[:start], line[end:]...)
		b.buffer[c.Y] = newLine

		// Ensure cursor is within bounds
		if c.X >= len(b.buffer[c.Y]) {
			c.X = len(b.buffer[c.Y])
			if c.X < 0 {
				c.X = 0
			}
		}

		// Handle syntax update
		if b.syntax != nil {
			deletedBytes := uint32(len(string(line[start:end])))
			oldColBytes := b.getLineByteOffset(line, start)
			newColBytes := b.getLineByteOffset(newLine, start)
			b.handleEdit(c.Y, start, deletedBytes, 0, c.Y, oldColBytes+deletedBytes, c.Y, newColBytes)
		}
	}

	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) deleteWordBackward() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 || b.PrimaryCursor().Y >= len(b.buffer) {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}
	line := b.buffer[b.PrimaryCursor().Y]
	if len(line) == 0 || b.PrimaryCursor().X == 0 {
		return
	}

	end := b.PrimaryCursor().X
	start := end

	// 1. Skip whitespace going back
	for start > 0 && (line[start-1] == ' ' || line[start-1] == '\t') {
		start--
	}

	// 2. Determine type of character before whitespace (or at cursor if no whitespace)
	if start > 0 {
		r := line[start-1]
		if e.isWordChar(r) {
			// On word characters: skip word characters
			for start > 0 && e.isWordChar(line[start-1]) {
				start--
			}
		} else {
			// On punctuation/other: skip those
			for start > 0 && !e.isWordChar(line[start-1]) && line[start-1] != ' ' && line[start-1] != '\t' {
				start--
			}
		}
	}

	// Delete from start to end
	newLine := append(line[:start], line[end:]...)
	b.buffer[b.PrimaryCursor().Y] = newLine
	b.PrimaryCursor().X = start

	// Handle syntax update
	if b.syntax != nil {
		deletedBytes := uint32(len(string(line[start:end])))
		oldColBytes := b.getLineByteOffset(line, start)
		newColBytes := b.getLineByteOffset(newLine, start)
		b.handleEdit(b.PrimaryCursor().Y, start, deletedBytes, 0, b.PrimaryCursor().Y, oldColBytes+deletedBytes, b.PrimaryCursor().Y, newColBytes)
	}

	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

// deleteWordBackwardFromBuffer removes the last word from the commandBuffer at cursor position.
func (e *Editor) deleteWordBackwardFromBuffer() {
	if e.commandCursorX == 0 {
		return
	}

	start := e.commandCursorX

	// Skip trailing whitespace.
	for start > 0 && (e.commandBuffer[start-1] == ' ' || e.commandBuffer[start-1] == '\t') {
		start--
	}

	// Delete word characters or punctuation.
	if start > 0 {
		r := e.commandBuffer[start-1]
		if e.isWordChar(r) {
			// Delete word characters.
			for start > 0 && e.isWordChar(e.commandBuffer[start-1]) {
				start--
			}
		} else {
			// Delete punctuation/other characters.
			for start > 0 && !e.isWordChar(e.commandBuffer[start-1]) && e.commandBuffer[start-1] != ' ' && e.commandBuffer[start-1] != '\t' {
				start--
			}
		}
	}

	// Remove the word from the buffer
	e.commandBuffer = append(e.commandBuffer[:start], e.commandBuffer[e.commandCursorX:]...)
	e.commandCursorX = start
}

func (e *Editor) changeWord() {
	b := e.activeBuffer()
	if b != nil && b.readOnly {
		e.message = "File is read-only"
		return
	}
	e.deleteWord(false)
	e.mode = ModeInsert
}

func (e *Editor) changeCharacter() {
	b := e.activeBuffer()
	if b != nil && b.readOnly {
		e.message = "File is read-only"
		return
	}
	e.DeleteChar()
	e.mode = ModeInsert
}

func (e *Editor) deleteToEndOfLine() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursors := e.getSortedCursorsDesc()
	for _, c := range cursors {
		if c.Y >= len(b.buffer) {
			continue
		}

		line := b.buffer[c.Y]
		if c.X >= len(line) {
			continue
		}

		// Save deleted text of the primary cursor to the clipboard
		if c == b.PrimaryCursor() {
			deletedText := line[c.X:]
			e.clipboard = make([]rune, len(deletedText))
			copy(e.clipboard, deletedText)
		}

		// Truncate the line at the cursor position
		deletedBytes := uint32(len(string(line[c.X:])))
		newLine := line[:c.X]
		b.buffer[c.Y] = newLine

		// Handle syntax update
		if b.syntax != nil {
			oldColBytes := b.getLineByteOffset(line, c.X)
			newColBytes := b.getLineByteOffset(newLine, c.X)
			b.handleEdit(c.Y, c.X, deletedBytes, 0, c.Y, oldColBytes+deletedBytes, c.Y, newColBytes)
		}
	}

	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) changeToEndOfLine() {
	b := e.activeBuffer()
	if b != nil && b.readOnly {
		e.message = "File is read-only"
		return
	}
	e.deleteToEndOfLine()
	e.mode = ModeInsert
}

// deleteInside removes text within a pair of delimiters (e.g., "", (), {}).
func (e *Editor) deleteInside(open, close rune) bool {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return false
	}
	if b.readOnly {
		e.message = "File is read-only"
		return false
	}
	line := b.buffer[b.PrimaryCursor().Y]
	if len(line) == 0 {
		return false
	}

	type pair struct {
		start, end int
	}
	var pairs []pair

	// Find all candidate delimiter pairs on the current line.
	if open == close {
		var indices []int
		for i, r := range line {
			if r == open {
				indices = append(indices, i)
			}
		}
		for i := 0; i+1 < len(indices); i += 2 {
			pairs = append(pairs, pair{indices[i], indices[i+1]})
		}
	} else {
		var stack []int
		for i, r := range line {
			if r == open {
				stack = append(stack, i)
			} else if r == close {
				if len(stack) > 0 {
					start := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					pairs = append(pairs, pair{start, i})
				}
			}
		}
	}

	// Find the smallest pair that strictly contains the cursor.
	var bestPair *pair
	for i := range pairs {
		p := &pairs[i]
		if b.PrimaryCursor().X >= p.start && b.PrimaryCursor().X <= p.end {
			if bestPair == nil || (p.start > bestPair.start) {
				bestPair = p
			}
		}
	}

	if bestPair == nil {
		for i := range pairs {
			p := &pairs[i]
			if p.start >= b.PrimaryCursor().X {
				if bestPair == nil || p.start < bestPair.start {
					bestPair = p
				}
			}
		}
	}

	if bestPair != nil && bestPair.end > bestPair.start+1 {
		start := bestPair.start
		end := bestPair.end
		deletedChars := line[start+1 : end]
		deletedBytes := uint32(len(string(deletedChars)))

		newLine := append(line[:start+1], line[end:]...)
		b.buffer[b.PrimaryCursor().Y] = newLine
		b.PrimaryCursor().X = start + 1

		if b.syntax != nil {
			oldColBytes := b.getLineByteOffset(line, start+1)
			newColBytes := b.getLineByteOffset(newLine, start+1)
			b.handleEdit(b.PrimaryCursor().Y, start+1, deletedBytes, 0, b.PrimaryCursor().Y, oldColBytes+deletedBytes, b.PrimaryCursor().Y, newColBytes)
		}
		if b.syntax != nil {
			b.syntax.Reparse([]byte(b.toString()))
		}
		e.markModified()
		return true
	}
	return false
}

func (e *Editor) changeInside(open, close rune) {
	if e.deleteInside(open, close) {
		e.mode = ModeInsert
	}
}

func (e *Editor) moveWordForward() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}

	// Helper to get char type: 0=space, 1=word, 2=punct
	getType := func(r rune) int {
		if r == ' ' || r == '\t' {
			return 0
		}
		if e.isWordChar(r) {
			return 1
		}
		return 2
	}

	// Process each cursor independently
	for i := range b.cursors {
		cursor := &b.cursors[i]

		if cursor.Y >= len(b.buffer) {
			cursor.Y = len(b.buffer) - 1
		}

		currentLine := b.buffer[cursor.Y]

		// 1. Skip current word/punct clump
		if cursor.X < len(currentLine) {
			startType := getType(currentLine[cursor.X])
			if startType != 0 {
				for cursor.X < len(currentLine) {
					if getType(currentLine[cursor.X]) != startType {
						break
					}
					cursor.X++
				}
			}
		}

		// 2. Skip whitespace
		for {
			// If at end of line, move to next line
			if cursor.X >= len(b.buffer[cursor.Y]) {
				if cursor.Y < len(b.buffer)-1 {
					cursor.Y++
					cursor.X = 0
					// Continue loop to check new line content
				} else {
					break // End of file
				}
			}

			line := b.buffer[cursor.Y]
			if len(line) == 0 {
				// Empty line, continue to next
				if cursor.Y < len(b.buffer)-1 {
					// We need to advance line manually here if we are on empty line
					// but only if we haven't just moved to it (which is handled by loop re-entry)
					// Actually, the check at top of loop handles line length check.
					// If line is empty, len is 0.
					// We just need to check if we are stuck.
					// If we are at X=0 on empty line, we should move to next line.

					// Let's rely on the loop condition:
					// if X >= len, it moves to next line.
					// if line is empty, len is 0. So X=0 >= 0 is true.
					// So it moves to next line immediately.
					// But we need to break if we found a word? No, empty line is not a word start.
					// So we continue.
				} else {
					break // EOF
				}
			} else {
				c := line[cursor.X]
				if c == ' ' || c == '\t' {
					cursor.X++
					continue
				}

				// Found start of next word
				break
			}
		}

		// Update preferred column
		cursor.PreferredCol = cursor.X
	}

	e.mergeCursors()
}

func (e *Editor) moveWordBackward() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}

	for i := range b.cursors {
		cursor := &b.cursors[i]

		// Helper to step back one char, wrapping lines
		stepBack := func() bool {
			if cursor.X > 0 {
				cursor.X--
				return true
			}
			if cursor.Y > 0 {
				cursor.Y--
				cursor.X = len(b.buffer[cursor.Y])
				if cursor.X > 0 {
					cursor.X--
				}
				return true
			}
			return false // Start of file
		}

		// 1. Move back 1 char initially
		if !stepBack() {
			continue
		}

		// 2. Skip whitespace going back
		for {
			line := b.buffer[cursor.Y]
			if len(line) == 0 {
				if !stepBack() {
					break
				}
				continue
			}

			c := line[cursor.X]
			if c == ' ' || c == '\t' {
				if !stepBack() {
					break
				}
				continue
			}
			break
		}

		// 3. We are on last char of a "word". Go to its start.
		line := b.buffer[cursor.Y]
		getType := func(r rune) int {
			if e.isWordChar(r) {
				return 1
			}
			return 2
		}

		if cursor.X < len(line) {
			targetType := getType(line[cursor.X])

			for cursor.X > 0 {
				prev := line[cursor.X-1]
				if prev == ' ' || prev == '\t' {
					break
				}
				if getType(prev) != targetType {
					break
				}
				cursor.X--
			}
		}

		cursor.PreferredCol = cursor.X
	}

	e.mergeCursors()
}

// deleteLine removes the current line and saves it to the clipboard.
func (e *Editor) deleteLine() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	line := b.buffer[b.PrimaryCursor().Y]
	e.clipboard = make([]rune, len(line)+1)
	copy(e.clipboard, line)
	e.clipboard[len(line)] = '\n'

	if len(b.buffer) == 1 {
		lineLen := uint32(len(string(b.buffer[0])))
		b.buffer[0] = []rune{}
		b.PrimaryCursor().X = 0

		if b.syntax != nil {
			b.handleEdit(0, 0, lineLen, 0, 0, lineLen, 0, 0)
		}
	} else {
		lineLen := uint32(len(string(b.buffer[b.PrimaryCursor().Y]))) + 1
		b.buffer = append(b.buffer[:b.PrimaryCursor().Y], b.buffer[b.PrimaryCursor().Y+1:]...)

		if b.syntax != nil {
			b.handleEdit(b.PrimaryCursor().Y, 0, lineLen, 0, b.PrimaryCursor().Y+1, 0, b.PrimaryCursor().Y, 0)
		}

		if b.PrimaryCursor().Y >= len(b.buffer) {
			b.PrimaryCursor().Y = len(b.buffer) - 1
		}
		b.PrimaryCursor().X = 0
	}
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

func (e *Editor) yankLine() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	line := b.buffer[b.PrimaryCursor().Y]
	e.clipboard = make([]rune, len(line)+1)
	copy(e.clipboard, line)
	e.clipboard[len(line)] = '\n'
}

func (e *Editor) pasteLine() {
	b := e.activeBuffer()
	if b == nil || len(e.clipboard) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	isLineWise := e.clipboard[len(e.clipboard)-1] == '\n'

	if isLineWise {
		content := e.clipboard[:len(e.clipboard)-1]
		parts := strings.Split(string(content), "\n")
		count := len(parts)

		newBuffer := make([][]rune, len(b.buffer)+count)
		copy(newBuffer[:b.PrimaryCursor().Y+1], b.buffer[:b.PrimaryCursor().Y+1])

		for i, part := range parts {
			newBuffer[b.PrimaryCursor().Y+1+i] = []rune(part)
		}

		copy(newBuffer[b.PrimaryCursor().Y+1+count:], b.buffer[b.PrimaryCursor().Y+1:])
		b.buffer = newBuffer

		b.PrimaryCursor().Y += count
		b.PrimaryCursor().X = 0
	} else {
		// Character-wise: paste after cursor
		fullText := string(e.clipboard)
		parts := strings.Split(fullText, "\n")

		if len(parts) == 1 {
			line := b.buffer[b.PrimaryCursor().Y]
			at := b.PrimaryCursor().X
			if len(line) > 0 {
				at++
			}
			if at > len(line) {
				at = len(line)
			}

			newLine := make([]rune, len(line)+len(e.clipboard))
			copy(newLine[:at], line[:at])
			copy(newLine[at:], e.clipboard)
			copy(newLine[at+len(e.clipboard):], line[at:])
			b.buffer[b.PrimaryCursor().Y] = newLine
			b.PrimaryCursor().X = at + len(e.clipboard) - 1
			if b.PrimaryCursor().X < 0 {
				b.PrimaryCursor().X = 0
			}
		} else {
			// Multi-line character-wise paste after cursor
			line := b.buffer[b.PrimaryCursor().Y]
			at := b.PrimaryCursor().X
			if len(line) > 0 {
				at++
			}
			if at > len(line) {
				at = len(line)
			}

			prefix := line[:at]
			suffix := line[at:]

			newLines := make([][]rune, len(parts))
			newLines[0] = append([]rune(nil), prefix...)
			newLines[0] = append(newLines[0], []rune(parts[0])...)

			for i := 1; i < len(parts)-1; i++ {
				newLines[i] = []rune(parts[i])
			}

			lastIndex := len(parts) - 1
			newLines[lastIndex] = []rune(parts[lastIndex])
			newLines[lastIndex] = append(newLines[lastIndex], suffix...)

			// Insert into buffer
			newBuffer := make([][]rune, len(b.buffer)+len(parts)-1)
			copy(newBuffer[:b.PrimaryCursor().Y], b.buffer[:b.PrimaryCursor().Y])
			copy(newBuffer[b.PrimaryCursor().Y:b.PrimaryCursor().Y+len(parts)], newLines)
			copy(newBuffer[b.PrimaryCursor().Y+len(parts):], b.buffer[b.PrimaryCursor().Y+1:])
			b.buffer = newBuffer

			// Move cursor to end of pasted text
			b.PrimaryCursor().Y = b.PrimaryCursor().Y + len(parts) - 1
			b.PrimaryCursor().X = len([]rune(parts[lastIndex]))
		}
	}
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) pasteLineAbove() {
	b := e.activeBuffer()
	if b == nil || len(e.clipboard) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	isLineWise := e.clipboard[len(e.clipboard)-1] == '\n'

	if isLineWise {
		content := e.clipboard[:len(e.clipboard)-1]
		parts := strings.Split(string(content), "\n")
		count := len(parts)

		newBuffer := make([][]rune, len(b.buffer)+count)
		copy(newBuffer[:b.PrimaryCursor().Y], b.buffer[:b.PrimaryCursor().Y])

		for i, part := range parts {
			newBuffer[b.PrimaryCursor().Y+i] = []rune(part)
		}

		copy(newBuffer[b.PrimaryCursor().Y+count:], b.buffer[b.PrimaryCursor().Y:])
		b.buffer = newBuffer

		b.PrimaryCursor().X = 0
	} else {
		// Character-wise: paste at cursor
		// Handle potential newlines in character-wise clipboard (e.g. from visual selection)
		fullText := string(e.clipboard)
		parts := strings.Split(fullText, "\n")

		if len(parts) == 1 {
			// Single line character-wise paste
			line := b.buffer[b.PrimaryCursor().Y]
			at := b.PrimaryCursor().X
			if at > len(line) {
				at = len(line)
			}

			newLine := make([]rune, len(line)+len(e.clipboard))
			copy(newLine[:at], line[:at])
			copy(newLine[at:], e.clipboard)
			copy(newLine[at+len(e.clipboard):], line[at:])
			b.buffer[b.PrimaryCursor().Y] = newLine
			b.PrimaryCursor().X = at + len(e.clipboard) - 1
			if b.PrimaryCursor().X < 0 {
				b.PrimaryCursor().X = 0
			}
		} else {
			// Multi-line character-wise paste
			line := b.buffer[b.PrimaryCursor().Y]
			prefix := line[:b.PrimaryCursor().X]
			suffix := line[b.PrimaryCursor().X:]

			newLines := make([][]rune, len(parts))
			newLines[0] = append([]rune(nil), prefix...)
			newLines[0] = append(newLines[0], []rune(parts[0])...)

			for i := 1; i < len(parts)-1; i++ {
				newLines[i] = []rune(parts[i])
			}

			lastIndex := len(parts) - 1
			newLines[lastIndex] = []rune(parts[lastIndex])
			newLines[lastIndex] = append(newLines[lastIndex], suffix...)

			// Insert into buffer
			newBuffer := make([][]rune, len(b.buffer)+len(parts)-1)
			copy(newBuffer[:b.PrimaryCursor().Y], b.buffer[:b.PrimaryCursor().Y])
			copy(newBuffer[b.PrimaryCursor().Y:b.PrimaryCursor().Y+len(parts)], newLines)
			copy(newBuffer[b.PrimaryCursor().Y+len(parts):], b.buffer[b.PrimaryCursor().Y+1:])
			b.buffer = newBuffer

			// Move cursor to end of pasted text
			b.PrimaryCursor().Y = b.PrimaryCursor().Y + len(parts) - 1
			b.PrimaryCursor().X = len([]rune(parts[lastIndex]))
		}
	}
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) duplicateLine() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	line := make([]rune, len(b.buffer[b.PrimaryCursor().Y]))
	copy(line, b.buffer[b.PrimaryCursor().Y])

	newBuffer := make([][]rune, len(b.buffer)+1)
	copy(newBuffer[:b.PrimaryCursor().Y+1], b.buffer[:b.PrimaryCursor().Y+1])
	newBuffer[b.PrimaryCursor().Y+1] = line
	copy(newBuffer[b.PrimaryCursor().Y+2:], b.buffer[b.PrimaryCursor().Y+1:])
	b.buffer = newBuffer

	b.PrimaryCursor().Y++
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) jumpToPrevEmptyLine() {
	e.pushJump()
	b := e.activeBuffer()
	if b == nil {
		return
	}
	// Search backwards from current line for an empty line
	for y := b.PrimaryCursor().Y - 1; y >= 0; y-- {
		if len(b.buffer[y]) == 0 {
			b.PrimaryCursor().Y = y
			b.PrimaryCursor().X = 0
			return
		}
	}
	e.jumpToTop()
}

func (e *Editor) jumpToNextEmptyLine() {
	e.pushJump()
	b := e.activeBuffer()
	if b == nil {
		return
	}
	// Search forwards from current line for an empty line
	for y := b.PrimaryCursor().Y + 1; y < len(b.buffer); y++ {
		if len(b.buffer[y]) == 0 {
			b.PrimaryCursor().Y = y
			b.PrimaryCursor().X = 0
			return
		}
	}
	e.jumpToBottom()
}

func (e *Editor) jumpToTop() {
	e.pushJump()
	b := e.activeBuffer()
	if b == nil {
		return
	}
	b.PrimaryCursor().Y = 0
	b.PrimaryCursor().X = 0
}

func (e *Editor) jumpToBottom() {
	e.pushJump()
	b := e.activeBuffer()
	if b == nil {
		return
	}
	b.PrimaryCursor().Y = len(b.buffer) - 1
	if b.PrimaryCursor().Y < 0 {
		b.PrimaryCursor().Y = 0
	}
	b.PrimaryCursor().X = 0
}

func (e *Editor) jumpToLineEnd() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	b.PrimaryCursor().X = len(b.buffer[b.PrimaryCursor().Y])
}

func (e *Editor) jumpToLineStart() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	b.PrimaryCursor().X = 0
}

func (e *Editor) jumpToFirstNonBlank() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	line := b.buffer[b.PrimaryCursor().Y]
	b.PrimaryCursor().X = 0
	for i, r := range line {
		if r != ' ' && r != '\t' {
			b.PrimaryCursor().X = i
			break
		}
	}
}

// saveState captures a deep copy of the current buffer and cursors for the undo stack.
func (e *Editor) saveState() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	// Deep copy the buffer to ensure historical states aren't mutated.
	bufferCopy := make([][]rune, len(b.buffer))
	for i, line := range b.buffer {
		lineCopy := make([]rune, len(line))
		copy(lineCopy, line)
		bufferCopy[i] = lineCopy
	}

	// Deep copy cursors.
	cursorsCopy := make([]Cursor, len(b.cursors))
	copy(cursorsCopy, b.cursors)

	b.undoStack = append(b.undoStack, HistoryState{
		buffer:  bufferCopy,
		cursors: cursorsCopy,
	})
	// Cap undo stack at 100 entries to prevent memory exhaustion.
	if len(b.undoStack) > 100 {
		b.undoStack = b.undoStack[1:]
	}
	// Clear the redo stack whenever a new action is performed.
	b.redoStack = []HistoryState{}
}

func (e *Editor) undo() {
	b := e.activeBuffer()
	if b == nil || len(b.undoStack) == 0 {
		return
	}

	// Save current state to redo stack
	bufferCopy := make([][]rune, len(b.buffer))
	for i, line := range b.buffer {
		lineCopy := make([]rune, len(line))
		copy(lineCopy, line)
		bufferCopy[i] = lineCopy
	}
	cursorsCopy := make([]Cursor, len(b.cursors))
	copy(cursorsCopy, b.cursors)

	b.redoStack = append(b.redoStack, HistoryState{
		buffer:  bufferCopy,
		cursors: cursorsCopy,
	})

	// Restore from undo stack
	state := b.undoStack[len(b.undoStack)-1]
	b.undoStack = b.undoStack[:len(b.undoStack)-1]
	b.buffer = state.buffer
	b.cursors = state.cursors

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) redo() {
	b := e.activeBuffer()
	if b == nil || len(b.redoStack) == 0 {
		return
	}

	// Save current state to undo stack
	bufferCopy := make([][]rune, len(b.buffer))
	for i, line := range b.buffer {
		lineCopy := make([]rune, len(line))
		copy(lineCopy, line)
		bufferCopy[i] = lineCopy
	}
	cursorsCopy := make([]Cursor, len(b.cursors))
	copy(cursorsCopy, b.cursors)

	b.undoStack = append(b.undoStack, HistoryState{
		buffer:  bufferCopy,
		cursors: cursorsCopy,
	})

	// Restore from redo stack
	state := b.redoStack[len(b.redoStack)-1]
	b.redoStack = b.redoStack[:len(b.redoStack)-1]
	b.buffer = state.buffer
	b.cursors = state.cursors

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

// JoinLines joins the current line with the next one.
func (e *Editor) JoinLines() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) <= 1 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	cursor := b.PrimaryCursor()
	if cursor.Y >= len(b.buffer)-1 {
		return // Last line, nothing to join
	}

	currentLine := b.buffer[cursor.Y]
	nextLine := b.buffer[cursor.Y+1]

	// Trim leading whitespace from next line
	trimIdx := 0
	for trimIdx < len(nextLine) && (nextLine[trimIdx] == ' ' || nextLine[trimIdx] == '\t') {
		trimIdx++
	}
	trimmedNextLine := nextLine[trimIdx:]

	// Determine if we need a space between lines
	needsSpace := true
	if len(currentLine) == 0 || (len(currentLine) > 0 && currentLine[len(currentLine)-1] == ' ') {
		needsSpace = false
	}
	if len(trimmedNextLine) == 0 {
		needsSpace = false
	}

	// Join lines
	newLine := make([]rune, 0, len(currentLine)+len(trimmedNextLine)+1)
	newLine = append(newLine, currentLine...)
	if needsSpace {
		newLine = append(newLine, ' ')
	}
	newLine = append(newLine, trimmedNextLine...)

	// Update buffer
	b.buffer[cursor.Y] = newLine
	b.buffer = append(b.buffer[:cursor.Y+1], b.buffer[cursor.Y+2:]...)

	// Set cursor position to the join point
	cursor.X = len(currentLine)
	if needsSpace {
		// Vim usually puts cursor on the space
	} else if cursor.X >= len(newLine) && len(newLine) > 0 {
		cursor.X = len(newLine) - 1
	}

	// Syntax update
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}
	e.markModified()
}

// getSelectionBounds returns the normalized coordinates (top-left to bottom-right) of the visual selection.
func (e *Editor) getSelectionBounds() (int, int, int, int) {
	b := e.activeBuffer()
	y1, x1, y2, x2 := e.visualStartY, e.visualStartX, b.PrimaryCursor().Y, b.PrimaryCursor().X

	// Normalize so (y1, x1) is always the "earlier" point in the file.
	if y1 > y2 || (y1 == y2 && x1 > x2) {
		y1, x1, y2, x2 = y2, x2, y1, x1
	}

	// Force line-wise bounds if in Visual Line mode.
	if e.mode == ModeVisualLine {
		x1 = 0
		if y2 < len(b.buffer) {
			x2 = len(b.buffer[y2])
			if x2 > 0 {
				x2-- // last character index
			} else {
				x2 = 0
			}
		}
	}

	return y1, x1, y2, x2
}

// ollamaComplete sends the selection to the Ollama AI and replaces it with the generated text.
func (e *Editor) ollamaComplete() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	if e.ollamaClient == nil || !e.ollamaClient.IsOnline {
		e.message = "Ollama is offline"
		return
	}

	y1, x1, y2, x2 := e.getSelectionBounds()

	// Extract selected text for the prompt.
	var selectedText strings.Builder
	for y := y1; y <= y2; y++ {
		line := b.buffer[y]
		if y == y1 && y == y2 {
			if x1 < len(line) {
				end := x2 + 1
				if end > len(line) {
					end = len(line)
				}
				selectedText.WriteString(string(line[x1:end]))
			}
		} else if y == y1 {
			if x1 < len(line) {
				selectedText.WriteString(string(line[x1:]))
			}
			selectedText.WriteRune('\n')
		} else if y == y2 {
			end := x2 + 1
			if end > len(line) {
				end = len(line)
			}
			selectedText.WriteString(string(line[:end]))
		} else {
			selectedText.WriteString(string(line))
			selectedText.WriteRune('\n')
		}
	}

	prompt := selectedText.String()
	if prompt == "" {
		return
	}

	// Read system prompt (template) from the embedded assets.
	instr, err := ContentFS.ReadFile("content/ollama.txt")
	if err == nil {
		prompt += "\n" + string(instr)
	}

	firstLine := strings.Split(prompt, "\n")[0]
	if len(firstLine) > 50 {
		firstLine = firstLine[:47] + "..."
	}
	e.message = fmt.Sprintf("Ollama is thinking about: %s", firstLine)
	e.draw()

	// Call the Ollama API.
	response, err := e.ollamaClient.Generate(prompt)
	if err != nil {
		e.message = fmt.Sprintf("Ollama error: %v", err)
		return
	}

	// Replace the visual selection with the AI's response.
	e.saveState()
	e.deleteVisualSelection()

	lines := strings.Split(strings.TrimSpace(response), "\n")

	at := b.PrimaryCursor().X
	currentLine := b.buffer[b.PrimaryCursor().Y]
	hasSuffix := at < len(currentLine)

	nextExists := b.PrimaryCursor().Y+1 < len(b.buffer)
	nextIsBlank := false
	if nextExists {
		nextIsBlank = len(b.buffer[b.PrimaryCursor().Y+1]) == 0
	}

	// Add formatting newlines if necessary.
	if hasSuffix {
		lines = append(lines, "", "")
	} else if !nextIsBlank {
		lines = append(lines, "")
	}

	if len(lines) == 1 {
		line := b.buffer[b.PrimaryCursor().Y]
		at := b.PrimaryCursor().X
		if at > len(line) {
			at = len(line)
		}

		respRunes := []rune(lines[0])
		newLine := make([]rune, len(line)+len(respRunes))
		copy(newLine[:at], line[:at])
		copy(newLine[at:], respRunes)
		copy(newLine[at+len(respRunes):], line[at:])
		b.buffer[b.PrimaryCursor().Y] = newLine
		b.PrimaryCursor().X = at + len(respRunes)
	} else {
		line := b.buffer[b.PrimaryCursor().Y]
		at := b.PrimaryCursor().X
		if at > len(line) {
			at = len(line)
		}

		prefix := line[:at]
		suffix := line[at:]

		newLines := make([][]rune, len(lines))
		for i, l := range lines {
			newLines[i] = []rune(l)
		}

		newLines[0] = append([]rune(string(prefix)), newLines[0]...)
		newLines[len(newLines)-1] = append(newLines[len(newLines)-1], suffix...)

		newBuffer := make([][]rune, len(b.buffer)+len(newLines)-1)
		copy(newBuffer[:b.PrimaryCursor().Y], b.buffer[:b.PrimaryCursor().Y])
		copy(newBuffer[b.PrimaryCursor().Y:], newLines)
		copy(newBuffer[b.PrimaryCursor().Y+len(newLines):], b.buffer[b.PrimaryCursor().Y+1:])
		b.buffer = newBuffer

		b.PrimaryCursor().Y = b.PrimaryCursor().Y + len(newLines) - 1
		b.PrimaryCursor().X = len(newLines[len(newLines)-1]) - len(suffix)
	}

	e.mode = ModeNormal
	e.markModified()
	e.message = "Ollama completion inserted (replaced selection)"

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) getSelection() []rune {
	b := e.activeBuffer()
	y1, x1, y2, x2 := e.getSelectionBounds()
	var selection []rune

	for y := y1; y <= y2; y++ {
		line := b.buffer[y]
		start := 0
		end := len(line)

		if e.mode == ModeVisualBlock {
			startX := x1
			endX := x2
			if startX > endX {
				startX, endX = endX, startX
			}
			start = startX
			end = endX + 1 // inclusive
		} else if e.mode != ModeVisualLine {
			if y == y1 {
				start = x1
			}
			if y == y2 {
				end = x2 + 1 // inclusive
			}
		}

		if start > len(line) {
			start = len(line)
		}
		if end > len(line) {
			end = len(line)
		}

		if start < end {
			selection = append(selection, line[start:end]...)
		}

		if (y < y2 || e.mode == ModeVisualLine) && e.mode != ModeVisualBlock {
			selection = append(selection, '\n')
		} else if e.mode == ModeVisualBlock && y < y2 {
			selection = append(selection, '\n')
		}
	}
	return selection
}

func (e *Editor) deleteVisualSelection() {
	b := e.activeBuffer()
	y1, x1, y2, x2 := e.getSelectionBounds()
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	// Copy to clipboard
	e.clipboard = e.getSelection()

	if e.mode == ModeVisualLine {
		// Remove all selected lines
		b.buffer = append(b.buffer[:y1], b.buffer[y2+1:]...)
		if len(b.buffer) == 0 {
			b.buffer = [][]rune{{}}
		}
		if y1 >= len(b.buffer) {
			y1 = len(b.buffer) - 1
		}
		b.PrimaryCursor().Y = y1
		b.PrimaryCursor().X = 0
	} else if e.mode == ModeVisualBlock {
		startX := x1
		endX := x2
		if startX > endX {
			startX, endX = endX, startX
		}

		for y := y1; y <= y2; y++ {
			if y < len(b.buffer) {
				line := b.buffer[y]
				s := startX
				e := endX + 1
				if s > len(line) {
					s = len(line)
				}
				if e > len(line) {
					e = len(line)
				}

				if s < e {
					newLine := append(line[:s], line[e:]...)
					b.buffer[y] = newLine
				}
			}
		}
		b.PrimaryCursor().Y = y1
		b.PrimaryCursor().X = startX
	} else {
		// Modify buffer for character-wise selection
		line1 := b.buffer[y1]
		line2 := b.buffer[y2]

		prefix := make([]rune, x1)
		copy(prefix, line1[:x1])

		suffix := []rune{}
		if x2+1 < len(line2) {
			suffix = make([]rune, len(line2)-(x2+1))
			copy(suffix, line2[x2+1:])
		}

		newLine := append(prefix, suffix...)
		b.buffer[y1] = newLine

		// Remove lines between
		if y1 != y2 {
			b.buffer = append(b.buffer[:y1+1], b.buffer[y2+1:]...)
		}

		b.PrimaryCursor().Y = y1
		b.PrimaryCursor().X = x1
	}

	e.mode = ModeNormal
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) yankVisualSelection() {
	e.clipboard = e.getSelection()
	e.mode = ModeNormal
}

func (e *Editor) changeVisualSelection() {
	b := e.activeBuffer()
	if b != nil && b.readOnly {
		e.message = "File is read-only"
		return
	}
	e.deleteVisualSelection()
	e.mode = ModeInsert
}

func (e *Editor) pasteVisualSelection() {
	if len(e.clipboard) == 0 {
		return
	}
	b := e.activeBuffer()
	if b != nil && b.readOnly {
		e.message = "File is read-only"
		return
	}
	// Save clipboard because deleteVisualSelection overwrites it
	tmpClipboard := make([]rune, len(e.clipboard))
	copy(tmpClipboard, e.clipboard)

	e.deleteVisualSelection()

	// Restore clipboard and paste
	e.clipboard = tmpClipboard
	e.pasteLineAbove()
}

func (e *Editor) toggleComment(y int) {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 || b.fileType == nil || b.fileType.Comment == "" {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}
	if y < 0 || y >= len(b.buffer) {
		return
	}

	line := b.buffer[y]
	if len(line) == 0 {
		return
	}

	comment := []rune(b.fileType.Comment)

	// Check if already commented at the beginning of the line
	isCommented := false
	if len(line) >= len(comment) {
		match := true
		for i, r := range comment {
			if line[i] != r {
				match = false
				break
			}
		}
		isCommented = match
	}

	var newLine []rune
	if isCommented {
		// Uncomment
		contentStart := len(comment)
		// Skip optional following space
		if contentStart < len(line) && line[contentStart] == ' ' {
			contentStart++
		}
		newLine = append(newLine, line[contentStart:]...)
	} else {
		// Comment
		newLine = append(newLine, comment...)
		newLine = append(newLine, ' ')
		newLine = append(newLine, line...)
	}

	b.buffer[y] = newLine
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) toggleCommentLine() {
	b := e.activeBuffer()
	if b != nil {
		e.toggleComment(b.PrimaryCursor().Y)
	}
}

func (e *Editor) commentVisualSelection() {
	y1, _, y2, _ := e.getSelectionBounds()
	for y := y1; y <= y2; y++ {
		e.toggleComment(y)
	}
	e.mode = ModeNormal
}

func (e *Editor) toggleCase(y, x int) (int, int) {
	b := e.activeBuffer()
	if b == nil || y < 0 || y >= len(b.buffer) {
		return y, x
	}
	line := b.buffer[y]
	if x < 0 || x >= len(line) {
		return y, x
	}

	r := line[x]
	if unicode.IsLower(r) {
		line[x] = unicode.ToUpper(r)
	} else if unicode.IsUpper(r) {
		line[x] = unicode.ToLower(r)
	}

	// Move cursor right
	newX := x + 1
	if newX >= len(line) {
		newX = len(line) - 1
		if newX < 0 {
			newX = 0
		}
	}

	return y, newX
}

func (e *Editor) ToggleCaseUnderCursor() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	e.saveState()
	b.PrimaryCursor().Y, b.PrimaryCursor().X = e.toggleCase(b.PrimaryCursor().Y, b.PrimaryCursor().X)
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

func (e *Editor) ToggleCaseVisualSelection() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	e.saveState()
	y1, x1, y2, x2 := e.getSelectionBounds()

	for y := y1; y <= y2; y++ {
		line := b.buffer[y]
		start := 0
		end := len(line) - 1
		if y == y1 {
			start = x1
		}
		if y == y2 {
			end = x2
		}

		for x := start; x <= end && x < len(line); x++ {
			r := line[x]
			if unicode.IsLower(r) {
				line[x] = unicode.ToUpper(r)
			} else if unicode.IsUpper(r) {
				line[x] = unicode.ToLower(r)
			}
		}
	}

	e.mode = ModeNormal
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}
}

// detectCommentPrefix checks if the given text starts with a known comment marker.
// Returns the comment prefix including trailing space if present, or empty string if not a comment.
// Uses Config.FormatterMarkers which can be customized in config.go.
func detectCommentPrefix(text string) string {
	for _, marker := range Config.FormatterMarkers {
		// Check for marker with space first (e.g., "// ")
		if strings.HasPrefix(text, marker+" ") {
			return marker + " "
		}
		// Then check for marker without space (e.g., "//")
		if strings.HasPrefix(text, marker) {
			return marker
		}
	}
	return ""
}

// formatText wraps text to 80 characters (gq-style formatting).
// It formats either the current line in normal mode or the selected lines in visual modes.
func (e *Editor) formatText() {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 {
		return
	}
	if b.readOnly {
		e.message = "File is read-only"
		return
	}

	var startLine, endLine int

	// Determine which lines to format based on current mode
	if e.mode == ModeNormal {
		// In normal mode, format only the current line
		startLine = b.PrimaryCursor().Y
		endLine = b.PrimaryCursor().Y
	} else if e.mode == ModeVisual || e.mode == ModeVisualLine {
		// In visual modes, format the selected lines
		startLine = e.visualStartY
		endLine = b.PrimaryCursor().Y
		if startLine > endLine {
			startLine, endLine = endLine, startLine
		}
	} else {
		return
	}

	e.saveState()

	const maxWidth = 80
	var newLines [][]rune

	// Process lines in groups (paragraphs) with the same indentation and comment prefix
	lineIdx := startLine
	for lineIdx <= endLine && lineIdx < len(b.buffer) {
		line := b.buffer[lineIdx]

		// Handle empty lines
		if len(line) == 0 {
			newLines = append(newLines, []rune{})
			lineIdx++
			continue
		}

		// Get leading whitespace (indentation) for this line
		indent := 0
		for indent < len(line) && unicode.IsSpace(line[indent]) {
			indent++
		}
		indentStr := line[:indent]

		// Get the text content (without indentation)
		content := line[indent:]
		contentStr := string(content)

		// Detect comment markers after indentation
		commentPrefix := detectCommentPrefix(contentStr)
		commentPrefixRunes := []rune(commentPrefix)

		// Collect all consecutive lines with the same indentation and comment prefix
		var paragraphText []string
		paragraphStartIdx := lineIdx

		for lineIdx <= endLine && lineIdx < len(b.buffer) {
			currentLine := b.buffer[lineIdx]

			// Stop at empty lines
			if len(currentLine) == 0 {
				break
			}

			// Check if this line has the same indentation
			currentIndent := 0
			for currentIndent < len(currentLine) && unicode.IsSpace(currentLine[currentIndent]) {
				currentIndent++
			}

			if currentIndent != indent {
				break
			}

			// Check if this line has the same comment prefix
			currentContent := currentLine[currentIndent:]
			currentContentStr := string(currentContent)
			currentCommentPrefix := detectCommentPrefix(currentContentStr)

			if currentCommentPrefix != commentPrefix {
				break
			}

			// Extract the actual text (after comment prefix)
			textContent := currentContentStr
			if len(commentPrefix) > 0 {
				textContent = currentContentStr[len(commentPrefix):]
			}

			paragraphText = append(paragraphText, strings.TrimSpace(textContent))
			lineIdx++
		}

		// Join all the text from the paragraph
		fullText := strings.Join(paragraphText, " ")

		// Split into words
		words := strings.Fields(fullText)
		if len(words) == 0 {
			// Just preserve the line structure if no words
			for i := paragraphStartIdx; i < lineIdx; i++ {
				newLines = append(newLines, b.buffer[i])
			}
			continue
		}

		// Now wrap the combined text
		wrapWidth := maxWidth - indent - len(commentPrefixRunes)
		if wrapWidth < 20 {
			wrapWidth = 20 // Minimum wrap width to avoid infinite loops
		}

		var wrappedLines [][]rune
		currentLine := make([]rune, 0)
		currentLine = append(currentLine, indentStr...)
		currentLine = append(currentLine, commentPrefixRunes...)

		for i, word := range words {
			wordRunes := []rune(word)

			// Calculate the length if we add this word
			// Account for space before word (except for first word on a line)
			spaceNeeded := 0
			if len(currentLine) > indent+len(commentPrefixRunes) {
				spaceNeeded = 1 // Need a space before the word
			}

			projectedLen := len(currentLine) + spaceNeeded + len(wordRunes)

			if projectedLen > maxWidth && len(currentLine) > indent+len(commentPrefixRunes) {
				// Adding this word would exceed max width, so start a new line
				wrappedLines = append(wrappedLines, currentLine)
				currentLine = make([]rune, 0)
				currentLine = append(currentLine, indentStr...)
				currentLine = append(currentLine, commentPrefixRunes...)
				currentLine = append(currentLine, wordRunes...)
			} else {
				// Add the word to the current line
				if len(currentLine) > indent+len(commentPrefixRunes) {
					currentLine = append(currentLine, ' ')
				}
				currentLine = append(currentLine, wordRunes...)
			}

			// If this is the last word, add the current line
			if i == len(words)-1 {
				wrappedLines = append(wrappedLines, currentLine)
			}
		}

		newLines = append(newLines, wrappedLines...)
	}

	// Replace the lines in the buffer
	if len(newLines) > 0 {
		newBuffer := make([][]rune, 0)
		newBuffer = append(newBuffer, b.buffer[:startLine]...)
		newBuffer = append(newBuffer, newLines...)
		if endLine+1 < len(b.buffer) {
			newBuffer = append(newBuffer, b.buffer[endLine+1:]...)
		}
		b.buffer = newBuffer

		// Adjust cursor position
		if b.PrimaryCursor().Y > len(b.buffer)-1 {
			b.PrimaryCursor().Y = len(b.buffer) - 1
		}
		if b.PrimaryCursor().Y >= 0 && b.PrimaryCursor().X > len(b.buffer[b.PrimaryCursor().Y]) {
			b.PrimaryCursor().X = len(b.buffer[b.PrimaryCursor().Y])
		}
	}

	e.mode = ModeNormal
	e.markModified()

	if b.syntax != nil {
		b.syntax.Parse([]byte(b.toString()))
	}

	e.message = "Text formatted"
}

// performSearch performs a linear case-insensitive search for a query string.
func (e *Editor) performSearch(query string, forward bool) {
	b := e.activeBuffer()
	if b == nil || len(b.buffer) == 0 || query == "" {
		return
	}

	queryLower := strings.ToLower(query)
	startY := b.PrimaryCursor().Y
	startX := b.PrimaryCursor().X

	dir := 1
	if !forward {
		dir = -1
	}

	y := startY
	firstLoop := true

	// Loop through the entire buffer once.
	for i := 0; i <= len(b.buffer); i++ {
		line := string(b.buffer[y])
		lineLower := strings.ToLower(line)

		matches := []int{}
		// Scan line for all occurrences.
		for pos := 0; pos < len(lineLower); {
			idx := strings.Index(lineLower[pos:], queryLower)
			if idx == -1 {
				break
			}
			matchPos := pos + idx
			matches = append(matches, matchPos)
			pos = matchPos + 1
		}

		if len(matches) > 0 {
			if forward {
				for _, m := range matches {
					// Ensure we skip the current cursor position on the first line.
					if firstLoop && m <= startX {
						continue
					}
					b.PrimaryCursor().Y = y
					b.PrimaryCursor().X = m
					return
				}
			} else {
				for j := len(matches) - 1; j >= 0; j-- {
					m := matches[j]
					if firstLoop && m >= startX {
						continue
					}
					b.PrimaryCursor().Y = y
					b.PrimaryCursor().X = m
					return
				}
			}
		}

		// Wrap around buffer boundaries.
		y += dir
		if y < 0 {
			y = len(b.buffer) - 1
		} else if y >= len(b.buffer) {
			y = 0
		}

		firstLoop = false
	}
}

func (e *Editor) findNext() {
	e.pushJump()
	e.performSearch(e.lastSearch, true)
}

func (e *Editor) findPrev() {
	e.pushJump()
	e.performSearch(e.lastSearch, false)
}

func (e *Editor) checkDiagnostics() {
	b := e.activeBuffer()
	if b == nil || b.lspClient == nil {
		return
	}

	e.addLog("LSP", "Checking diagnostics...")

	// Send current buffer content to LSP
	content := e.bufferToString(b.buffer)
	if err := b.lspClient.SendDidChange(content); err != nil {
		e.addLog("LSP", fmt.Sprintf("didChange error: %v", err))
		return
	}

	// Diagnostics will be updated asynchronously when clangd sends publishDiagnostics
	// The background readMessages goroutine handles this automatically
	// Get current diagnostics (may be from previous check)
	b.diagnostics = b.lspClient.GetDiagnostics()
	e.addLog("LSP", fmt.Sprintf("Current diagnostics: %d", len(b.diagnostics)))
}

func (e *Editor) deleteCurrentBuffer() {
	if len(e.buffers) == 0 {
		return
	}

	// Shutdown LSP client if active
	b := e.activeBuffer()
	if b != nil && b.lspClient != nil {
		b.lspClient.Shutdown()
	}

	// Remove the current buffer
	e.buffers = append(e.buffers[:e.activeBufferIndex], e.buffers[e.activeBufferIndex+1:]...)

	// Adjust active buffer index
	if len(e.buffers) == 0 {
		// No more buffers, create an empty one
		defaultType := fileTypes[len(fileTypes)-1]
		e.buffers = append(e.buffers, &Buffer{
			buffer:    [][]rune{{}},
			undoStack: []HistoryState{},
			redoStack: []HistoryState{},
			fileType:  defaultType,
		})
		e.activeBufferIndex = 0
	} else if e.activeBufferIndex >= len(e.buffers) {
		e.activeBufferIndex = len(e.buffers) - 1
	}
}

// drawStatusBar renders the bottom-aligned information bar showing file details and editor state.
func (e *Editor) drawStatusBar(statusY int) {
	w, _ := termbox.Size()
	b := e.activeBuffer()
	if b == nil {
		return
	}

	modeStr := "UNKNOWN"

	// Fill background for the entire status line.
	for x := 0; x < w; x++ {
		fg, bg := GetThemeColor(ColorStatusBar)
		termbox.SetCell(x, statusY, ' ', fg, bg)
	}

	// Draw the primary mode indicator.
	var fg, bg termbox.Attribute
	switch e.mode {
	case ModeInsert:
		modeStr = "INSERT"
		fg, bg = GetThemeColor(ColorInsertMode)
	case ModeVisual, ModeVisualLine, ModeVisualBlock:
		modeStr = "VISUAL"
		fg, bg = GetThemeColor(ColorVisualMode)
	case ModeFuzzy:
		switch e.fuzzyType {
		case FuzzyModeFile:
			modeStr = "FILES"
			fg, bg = GetThemeColor(ColorFuzzyModeFiles)
		case FuzzyModeBuffer:
			modeStr = "BUFFERS"
			fg, bg = GetThemeColor(ColorFuzzyModeBuffers)
		case FuzzyModeWarning:
			modeStr = "WARNINGS"
			fg, bg = GetThemeColor(ColorFuzzyModeWarnings)
		default:
			modeStr = "FUZZY"
			fg, bg = GetThemeColor(ColorNormalMode)
		}
	default:
		modeStr = "NORMAL"
		fg, bg = GetThemeColor(ColorNormalMode)
	}

	termbox.SetCell(0, statusY, ' ', fg, bg)
	for i, r := range modeStr {
		termbox.SetCell(i+1, statusY, r, fg, bg)
	}
	termbox.SetCell(len(modeStr)+1, statusY, ' ', fg, bg)

	// Draw filename and modification status.
	fileStr := "[no file]"
	if b.filename != "" {
		fileStr = b.filename
	}
	if b.modified {
		fileStr += " [+]"
	}
	if b.readOnly {
		fileStr += " (read-only)"
	}
	fileX := len(modeStr) + 2 + 1
	for i, r := range fileStr {
		fg, bg := GetThemeColor(ColorStatusBar)
		termbox.SetCell(fileX+i, statusY, r, fg, bg)
	}

	// Draw cursor coordinates and file metadata.
	lineNum := b.PrimaryCursor().Y + 1
	visualCol := e.bufferToVisual(b.buffer[b.PrimaryCursor().Y], b.PrimaryCursor().X) + 1
	totalLines := len(b.buffer)
	percent := 0
	if totalLines > 0 {
		percent = (lineNum * 100) / totalLines
	}
	fileTypeStr := "text"
	if b.fileType != nil {
		fileTypeStr = strings.ToLower(b.fileType.Name)
	}
	statusRight := fmt.Sprintf("(%s) [%d/%d] %d,%d %d%% ", fileTypeStr, e.activeBufferIndex+1, len(e.buffers), lineNum, visualCol, percent)
	rightPositionWidth := 6
	rightX := w - len(statusRight) - rightPositionWidth
	for i, r := range statusRight {
		fg, bg := GetThemeColor(ColorStatusBar)
		termbox.SetCell(rightX+i, statusY, r, fg, bg)
	}

	// Draw connectivity status for LSP and Ollama.
	lspColor := ColorLSPStatusDisconnected
	if b.lspClient != nil {
		lspColor = ColorLSPStatusConnected
	}
	fgL, bgL := GetThemeColor(lspColor)
	for i, r := range " L " {
		termbox.SetCell(w-6+i, statusY, r, fgL, bgL)
	}

	ollamaColor := ColorOllamaStatusDisconnected
	if e.ollamaClient != nil && e.ollamaClient.IsOnline {
		ollamaColor = ColorOllamaStatusConnected
	}
	fgO, bgO := GetThemeColor(ollamaColor)
	for i, r := range " O " {
		termbox.SetCell(w-3+i, statusY, r, fgO, bgO)
	}
}

func (e *Editor) drawCommandBar(cmdY int) {
	w, _ := termbox.Size()
	for x := 0; x < w; x++ {
		fg, bg := GetThemeColor(ColorDefault)
		termbox.SetCell(x, cmdY, ' ', fg, bg)
	}

	prompt := ""
	buffer := []rune{}
	startX := 0
	if e.mode == ModeCommand {
		prompt = ":"
		buffer = e.commandBuffer
	} else if e.mode == ModeFuzzy {
		prompt = "> "
		buffer = e.fuzzyBuffer
		startX = 1
	} else if e.mode == ModeFind {
		prompt = "/"
		buffer = e.findBuffer
	} else if e.mode == ModeReplace {
		prompt = "replace: "
		buffer = e.replaceInput
	} else if e.message != "" {
		// Draw transient message
		for i, r := range e.message {
			if i >= w {
				break
			}
			fg, bg := GetThemeColor(ColorDefault)
			termbox.SetCell(i, cmdY, r, fg, bg)
		}
		return
	} else {
		// Show LSP diagnostics when not in command/fuzzy/find mode
		b := e.activeBuffer()
		if b != nil && len(b.diagnostics) > 0 {
			// Count errors and warnings
			errorCount := 0
			for _, d := range b.diagnostics {
				if d.Severity == 1 { // Error
					errorCount++
				}
			}

			// Show diagnostic summary
			diagStr := ""
			if errorCount > 0 {
				diagStr = fmt.Sprintf("%d error(s): ", errorCount)
			} else {
				diagStr = fmt.Sprintf("%d diag(s): ", len(b.diagnostics))
			}

			// Add first error message (truncated if too long)
			// Make it as long as possible as width of the terminal allows
			if len(b.diagnostics) > 0 {
				firstMsg := b.diagnostics[0].Message
				maxMsgLen := w - len(diagStr)
				if len(firstMsg) > maxMsgLen {
					firstMsg = firstMsg[:maxMsgLen-3] + "..."
				}
				diagStr += firstMsg
			}

			// Draw diagnostic text using theme colors
			fg, _ := GetThemeColor(ColorDiagSummaryError)
			if errorCount == 0 {
				fg, _ = GetThemeColor(ColorDiagSummaryWarning)
			}

			for i, r := range diagStr {
				if i >= w {
					break
				}
				_, bg := GetThemeColor(ColorDefault)
				termbox.SetCell(i, cmdY, r, fg, bg)
			}
			return
		}
	}

	// Draw prompt
	for i, r := range prompt {
		fg, bg := GetThemeColor(ColorDefault)
		termbox.SetCell(startX+i, cmdY, r, fg, bg)
	}

	// Draw buffer content
	for i, r := range buffer {
		fg, bg := GetThemeColor(ColorDefault)
		termbox.SetCell(startX+len(prompt)+i, cmdY, r, fg, bg)
	}
}

func (e *Editor) highlightLine(lineIdx int, line []rune) ([]termbox.Attribute, []termbox.Attribute) {
	fgAttrs := make([]termbox.Attribute, len(line))
	bgAttrs := make([]termbox.Attribute, len(line))

	// specific default color for text
	defaultFg, defaultBg := GetThemeColor(ColorDefault)

	for i := range fgAttrs {
		fgAttrs[i] = defaultFg
		bgAttrs[i] = defaultBg
	}

	b := e.activeBuffer()
	if b != nil && b.syntax != nil {
		attrs := b.syntax.Highlight(lineIdx, line)
		// SyntaxHighlighter returns FG colors.
		// We trust it to return a slice of the same length as line (or we handle potential mismatch if necessary,
		// but the current implementation of Highlight seems to handle checks).
		// Overwrite default FGs with syntax FGs where they differ from default?
		// Or just replace entirely? syntax.Highlight initializes with defaultFg.
		// So we can just use it.
		fgAttrs = attrs
	}

	return fgAttrs, bgAttrs
}

func matchesKeyword(runes []rune, start int, keyword string) bool {
	if start+len(keyword) > len(runes) {
		return false
	}
	for i, ch := range keyword {
		if runes[start+i] != ch {
			return false
		}
	}
	return true
}

func isWordStart(line []rune, i int) bool {
	if i == 0 {
		return true
	}
	r := line[i-1]
	// If previous char is not a word char, then this is start of word (assuming current is word char)
	return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
}

// draw is the main UI rendering loop.
func (e *Editor) draw() {
	_, defaultBg := GetThemeColor(ColorDefault)
	termbox.Clear(termbox.ColorDefault, defaultBg)
	w, h := termbox.Size()
	b := e.activeBuffer()
	if b == nil {
		termbox.Flush()
		return
	}

	textWidth := w - Config.GutterWidth
	visibleHeight := h - 2
	if e.mode == ModeFuzzy {
		visibleHeight = h - 2 - Config.FuzzyFinderHeight
	}

	// Vertical scroll management.
	if b.PrimaryCursor().Y < b.scrollY {
		b.scrollY = b.PrimaryCursor().Y
	}
	if b.PrimaryCursor().Y >= b.scrollY+visibleHeight {
		b.scrollY = b.PrimaryCursor().Y - visibleHeight + 1
	}

	// Horizontal scroll management.
	visualCursorX := e.bufferToVisual(b.buffer[b.PrimaryCursor().Y], b.PrimaryCursor().X)
	if visualCursorX < b.scrollX {
		b.scrollX = visualCursorX
	}
	if visualCursorX >= b.scrollX+textWidth {
		b.scrollX = visualCursorX - textWidth + 1
	}

	// Optimized mapping for faster cursor lookup during rendering.
	cursorMap := make(map[int]map[int]bool)
	for _, c := range b.cursors {
		if _, ok := cursorMap[c.Y]; !ok {
			cursorMap[c.Y] = make(map[int]bool)
		}
		cursorMap[c.Y][c.X] = true
	}

	for screenY := 0; screenY < visibleHeight; screenY++ {
		bufferY := screenY + b.scrollY
		if bufferY < len(b.buffer) {
			// LSP diagnostic sign rendering.
			diagSign := ' '
			diagColor, diagBg := GetThemeColor(ColorDefault)
			if b.diagnostics != nil {
				for _, diag := range b.diagnostics {
					if diag.Range.Start.Line == bufferY {
						if diag.Severity == 1 {
							diagSign = 'E'
							diagColor, diagBg = GetThemeColor(ColorGutterSignError)
						} else if diag.Severity == 2 && diagSign != 'E' {
							diagSign = 'W'
							diagColor, diagBg = GetThemeColor(ColorGutterSignWarning)
						} else if diag.Severity == 3 && diagSign != 'E' {
							diagSign = 'I'
							diagColor, diagBg = GetThemeColor(ColorGutterSignInfo)
						} else if diag.Severity == 4 && diagSign != 'E' {
							diagSign = 'H'
							diagColor, diagBg = GetThemeColor(ColorGutterSignHint)
						}
					}
				}
			}

			termbox.SetCell(0, screenY, diagSign, diagColor, diagBg)
			termbox.SetCell(1, screenY, ' ', diagBg, diagBg)

			// Gutter line number rendering.
			lineNum := strconv.Itoa(bufferY + 1)
			gutterFg, gutterBg := GetThemeColor(ColorGutterLineNumber)
			for i, r := range lineNum {
				termbox.SetCell(Config.GutterWidth-len(lineNum)-1+i, screenY, r, gutterFg, gutterBg)
			}

			// Text highlighting and rendering block.
			var fgAttrs []termbox.Attribute
			var bgAttrs []termbox.Attribute
			if b.fileType != nil && b.fileType.Name != "Default" {
				fgAttrs, bgAttrs = e.highlightLine(bufferY, b.buffer[bufferY])
			} else {
				fgAttrs = make([]termbox.Attribute, len(b.buffer[bufferY]))
				bgAttrs = make([]termbox.Attribute, len(b.buffer[bufferY]))
				for k := range fgAttrs {
					fgAttrs[k], bgAttrs[k] = GetThemeColor(ColorDefault)
				}
			}

			_, bg := GetThemeColor(ColorDefault)
			if bufferY == b.PrimaryCursor().Y {
				_, bg = GetThemeColor(ColorHighlightedLine)
				for x := 0; x < textWidth; x++ {
					fg, _ := GetThemeColor(ColorDefault)
					termbox.SetCell(x+Config.GutterWidth, screenY, ' ', fg, bg)
				}
			}

			inVisual := e.mode == ModeVisual || e.mode == ModeVisualLine || e.mode == ModeVisualBlock
			var vStartY, vStartX, vEndY, vEndX int
			if inVisual {
				y1, x1, y2, x2 := e.getSelectionBounds()
				vStartY, vStartX, vEndY, vEndX = y1, x1, y2, x2
				if e.mode == ModeVisualBlock {
					if vStartX > vEndX {
						vStartX, vEndX = vEndX, vStartX
					}
				}
			}

			searchMatches := []bool{}
			if e.lastSearch != "" {
				searchMatches = make([]bool, len(b.buffer[bufferY]))
				lineRunes := b.buffer[bufferY]
				queryRunes := []rune(strings.ToLower(e.lastSearch))
				queryLen := len(queryRunes)

				for i := 0; i <= len(lineRunes)-queryLen; i++ {
					match := true
					for j := 0; j < queryLen; j++ {
						if unicode.ToLower(lineRunes[i+j]) != queryRunes[j] {
							match = false
							break
						}
					}
					if match {
						for k := 0; k < queryLen; k++ {
							searchMatches[i+k] = true
						}
					}
				}
			}

			visualX := 0
			for idx, r := range b.buffer[bufferY] {
				width := e.visualWidth(r, visualX)

				charBg := bg
				isVisualSelected := false
				if inVisual {
					if e.mode == ModeVisualBlock {
						if bufferY >= vStartY && bufferY <= vEndY {
							if idx >= vStartX && idx < vEndX+1 {
								isVisualSelected = true
							}
						}
					} else if e.mode == ModeVisualLine {
						if bufferY >= vStartY && bufferY <= vEndY {
							isVisualSelected = true
						}
					} else {
						if bufferY > vStartY && bufferY < vEndY {
							isVisualSelected = true
						} else if bufferY == vStartY && bufferY == vEndY {
							if idx >= vStartX && idx < vEndX+1 {
								isVisualSelected = true
							}
						} else if bufferY == vStartY {
							if idx >= vStartX {
								isVisualSelected = true
							}
						} else if bufferY == vEndY {
							if idx < vEndX+1 {
								isVisualSelected = true
							}
						}
					}
				}

				isCursor := false
				if cm, ok := cursorMap[bufferY]; ok {
					if cm[idx] {
						isCursor = true
					}
				}

				if isVisualSelected {
					selFg, selBg := GetThemeColor(ColorVisualModeSelection)
					charBg = selBg
					if idx < len(fgAttrs) {
						fgAttrs[idx] = selFg
					}
				}

				if isCursor {
					charBg, fgAttrs[idx] = fgAttrs[idx], charBg
					if charBg == fgAttrs[idx] {
						_, forcedBg := GetThemeColor(ColorCursor)
						charBg = forcedBg
						fgAttrs[idx] = termbox.ColorWhite
					}
				}

				if !isVisualSelected && len(searchMatches) > idx && searchMatches[idx] {
					searchMatchFg, searchMatchBg := GetThemeColor(ColorSearchMatch)
					charBg = searchMatchBg
					fgAttrs[idx] = searchMatchFg
				}

				if e.mode == ModeReplace {
					for _, match := range e.replaceMatches {
						if match.startLine == bufferY && idx >= match.startCol && idx < match.endCol {
							replaceMatchFg, replaceMatchBg := GetThemeColor(ColorReplaceMatch)
							charBg = replaceMatchBg
							fgAttrs[idx] = replaceMatchFg
							break
						}
					}
				}

				if !isVisualSelected && charBg == bg && len(bgAttrs) > idx && bgAttrs[idx] != defaultBg {
					charBg = bgAttrs[idx]
				}

				for i := 0; i < width; i++ {
					screenX := visualX + i - b.scrollX
					if screenX >= 0 && screenX < textWidth {
						char := r
						if r == '\t' {
							char = ' '
						}
						termbox.SetCell(screenX+Config.GutterWidth, screenY, char, fgAttrs[idx], charBg)
					}
				}
				visualX += width
			}

			if e.mode == ModeVisualLine && bufferY >= vStartY && bufferY <= vEndY {
				_, visualModeLineBg := GetThemeColor(ColorVisualModeSelection)
				for x := visualX - b.scrollX; x < textWidth; x++ {
					if x >= 0 {
						termbox.SetCell(x+Config.GutterWidth, screenY, ' ', termbox.ColorDefault, visualModeLineBg)
					}
				}
			}
		} else {
			fg, bg := GetThemeColor(ColorEmptyLineMarker)
			termbox.SetCell(0, screenY, '~', fg, bg)
		}
	}

	if !e.introDismissed && b.filename == "" && len(b.buffer) == 1 && len(b.buffer[0]) == 0 && !b.modified && e.mode != ModeInsert {
		e.drawIntro()
	}

	if e.mode == ModeFuzzy {
		statusY := h - 2 - Config.FuzzyFinderHeight
		fuzzyY := h - 1 - Config.FuzzyFinderHeight
		cmdY := h - 1

		e.drawStatusBar(statusY)
		e.drawFuzzyFinder(fuzzyY, Config.FuzzyFinderHeight)
		e.drawCommandBar(cmdY)
	} else {
		e.drawStatusBar(h - 2)
		e.drawCommandBar(h - 1)
	}

	if e.showDebugLog {
		e.drawDebugDiagnostics()
	}

	if e.showHover {
		e.drawHoverPopup()
	}

	if e.showAutocomplete {
		e.drawAutocompletePopup()
	}

	// Synchronize terminal cursor with editor focus.
	if e.mode == ModeCommand {
		termbox.SetCursor(e.commandCursorX+1, h-1)
	} else if e.mode == ModeFuzzy {
		termbox.SetCursor(len(e.fuzzyBuffer)+3, h-1)
	} else if e.mode == ModeFind {
		termbox.SetCursor(len(e.findBuffer)+1, h-1)
	} else if e.mode == ModeReplace {
		termbox.SetCursor(len(e.replaceInput)+9, h-1)
	} else {
		termbox.SetCursor(visualCursorX-b.scrollX+Config.GutterWidth, b.PrimaryCursor().Y-b.scrollY)
	}
	termbox.Flush()
}

func (e *Editor) drawDebugDiagnostics() {
	w, h := termbox.Size()
	b := e.activeBuffer()
	if b == nil {
		return
	}

	startX := 0
	startY := h - 22

	// Draw window background
	for y := startY; y < startY+w && y < h-2; y++ {
		for x := startX; x < w; x++ {
			fg, bg := GetThemeColor(ColorDebugWindow)
			termbox.SetCell(x, y, ' ', fg, bg)
		}
	}

	// Draw window title
	title := "[DEBUG LOG]"
	titleX := startX + (w-len(title))/2
	for i, r := range title {
		fg, bg := GetThemeColor(ColorDebugTitle)
		termbox.SetCell(titleX+i, startY, r, fg, bg)
	}

	// Prepare content lines
	contentLines := []string{}

	// Add LSP status
	if b.lspClient != nil {
		contentLines = append(contentLines, fmt.Sprintf("LSP: Active (%s)", filepath.Base(b.filename)))
		contentLines = append(contentLines, fmt.Sprintf("Diags: %d", len(b.diagnostics)))

		// Show first few diagnostics
		for i, diag := range b.diagnostics {
			if i >= 3 {
				contentLines = append(contentLines, "  ...")
				break
			}
			sevStr := "?"
			switch diag.Severity {
			case 1:
				sevStr = "E"
			case 2:
				sevStr = "W"
			case 3:
				sevStr = "I"
			case 4:
				sevStr = "H"
			}
			msg := fmt.Sprintf("  [%s] L%d: %s", sevStr, diag.Range.Start.Line+1, diag.Message)
			if len(msg) > w-2 {
				msg = msg[:w-5] + "..."
			}
			contentLines = append(contentLines, msg)
		}
		contentLines = append(contentLines, "---")
	}

	// Add recent log messages (last 8)
	startLog := 0
	if len(e.logMessages) > Config.NumLogsInDebugWindow {
		startLog = len(e.logMessages) - Config.NumLogsInDebugWindow
	}
	for i := startLog; i < len(e.logMessages); i++ {
		msg := e.logMessages[i]
		if len(msg) > w-2 {
			msg = msg[:w-5] + "..."
		}
		contentLines = append(contentLines, msg)
	}

	// Draw content
	for i, line := range contentLines {
		if i >= w-2 {
			break
		}
		y := startY + 1 + i
		x := startX + 1
		for j, r := range line {
			if x+j >= w {
				break
			}
			fg, bg := GetThemeColor(ColorDebugWindow)
			termbox.SetCell(x+j, y, r, fg, bg)
		}
	}
}

func (e *Editor) drawFuzzyFinder(startY int, fuzzyHeight int) {
	w, _ := termbox.Size()

	// Draw results
	for i := 0; i < fuzzyHeight; i++ {
		resultIdx := i + e.fuzzyScroll
		if resultIdx >= len(e.fuzzyResults) {
			break
		}

		file := e.fuzzyResults[resultIdx]
		y := startY + fuzzyHeight - 1 - i
		fg, bg := GetThemeColor(ColorFuzzyResult)

		if resultIdx == e.fuzzyIndex {
			// Highlight the entire selected line
			selFg, selBg := GetThemeColor(ColorFuzzySelected)
			for x := 0; x < w; x++ {
				termbox.SetCell(x, y, ' ', selFg, selBg)
			}
			fg, bg = selFg, selBg
			file = " > " + file
		} else {
			file = "   " + file
		}

		for x, r := range file {
			if x < w {
				termbox.SetCell(x, y, r, fg, bg)
			}
		}
	}
}

func (e *Editor) centerScreen() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	_, h := termbox.Size()
	visibleHeight := h - 2

	// Calculate target scroll to center current line
	targetScrollY := b.PrimaryCursor().Y - (visibleHeight / 2)

	// Don't scroll beyond buffer bounds
	if targetScrollY < 0 {
		targetScrollY = 0
	}
	if targetScrollY > len(b.buffer)-visibleHeight {
		targetScrollY = len(b.buffer) - visibleHeight
	}
	if targetScrollY < 0 {
		targetScrollY = 0
	}

	b.scrollY = targetScrollY
}

func (e *Editor) addCursorAbove() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	primary := b.PrimaryCursor()
	if primary.Y > 0 {
		b.AddCursor(primary.X, primary.Y-1)
	}
}

func (e *Editor) addCursorBelow() {
	b := e.activeBuffer()
	if b == nil {
		return
	}

	// Find the cursor with the highest Y (lowest visual position)
	maxY := -1
	targetX := -1

	for _, c := range b.cursors {
		if c.Y > maxY {
			maxY = c.Y
			targetX = c.X
			// If this cursor has a preferred column, use that instead of current X
			// This helps when moving through shorter lines
			if c.PreferredCol > targetX {
				targetX = c.PreferredCol
			}
		}
	}

	if maxY < len(b.buffer)-1 {
		b.AddCursor(targetX, maxY+1)
	}
}

func (e *Editor) clearSecondaryCursors() {
	b := e.activeBuffer()
	if b == nil {
		return
	}
	b.ClearCursors()
}

func (e *Editor) drawHoverPopup() {
	if !e.showHover || e.hoverContent == "" {
		return
	}

	w, _ := termbox.Size()
	b := e.activeBuffer()
	if b == nil {
		return
	}

	lines := strings.Split(e.hoverContent, "\n")
	maxWidth := 0
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	// Cap width to terminal width
	if maxWidth > w-10 {
		maxWidth = w - 10
	}

	paddingX := 2
	paddingY := 1
	popupWidth := maxWidth + (paddingX * 2)
	popupHeight := len(lines) + (paddingY * 2)

	// Calculate position (above cursor)
	visualCursorX := e.bufferToVisual(b.buffer[b.PrimaryCursor().Y], b.PrimaryCursor().X)
	cursorScreenX := visualCursorX - b.scrollX + Config.GutterWidth
	cursorScreenY := b.PrimaryCursor().Y - b.scrollY

	startX := cursorScreenX
	startY := cursorScreenY - popupHeight

	// Adjust if out of bounds
	if startY < 0 {
		startY = cursorScreenY + 1
	}
	if startX+popupWidth > w {
		startX = w - popupWidth
	}
	if startX < 0 {
		startX = 0
	}

	fg, bg := GetThemeColor(ColorHoverWindow)
	// Draw background and content
	for y := 0; y < popupHeight; y++ {
		for x := 0; x < popupWidth; x++ {
			termbox.SetCell(startX+x, startY+y, ' ', fg, bg)
		}
	}

	// Draw content lines
	for i, line := range lines {
		if i >= len(lines) {
			break
		}
		y := startY + paddingY + i
		for j, r := range line {
			if j >= maxWidth {
				break
			}
			if startX+paddingX+j < w {
				termbox.SetCell(startX+paddingX+j, y, r, fg, bg)
			}
		}
	}
}

// triggerHover initiates an LSP hover request for the current cursor position.
func (e *Editor) triggerHover() {
	b := e.activeBuffer()
	if b == nil || b.lspClient == nil {
		return
	}

	e.message = "Requesting signature..."
	e.draw()

	cursor := b.PrimaryCursor()
	content, err := b.lspClient.Hover(cursor.Y, cursor.X)
	if err != nil {
		e.message = fmt.Sprintf("LSP Hover error: %v", err)
		return
	}

	e.hoverContent = content
	e.showHover = true
}

// triggerAutocomplete initiates an LSP completion request for the current cursor position.
func (e *Editor) triggerAutocomplete() {
	b := e.activeBuffer()
	if b == nil || b.lspClient == nil {
		return
	}

	e.message = "Requesting completions..."
	e.draw()

	cursor := b.PrimaryCursor()
	items, err := b.lspClient.Completion(cursor.Y, cursor.X)
	if err != nil {
		e.message = fmt.Sprintf("LSP Completion error: %v", err)
		return
	}

	if len(items) == 0 {
		e.message = "No completions available"
		return
	}

	e.autocompleteItems = items
	e.autocompleteIndex = 0
	e.autocompleteScroll = 0
	e.showAutocomplete = true
	e.message = ""
}

func (e *Editor) drawAutocompletePopup() {
	if !e.showAutocomplete || len(e.autocompleteItems) == 0 {
		return
	}

	w, h := termbox.Size()
	b := e.activeBuffer()
	if b == nil {
		return
	}

	// Calculate max label width for alignment
	maxLabelWidth := 0
	for _, item := range e.autocompleteItems {
		if len(item.Label) > maxLabelWidth {
			maxLabelWidth = len(item.Label)
		}
	}

	// Calculate total width: label + separator + detail
	maxWidth := 0
	for _, item := range e.autocompleteItems {
		displayText := item.Label
		if item.Detail != "" {
			// Pad label to align, then add arrow and detail
			padding := maxLabelWidth - len(item.Label)
			displayText = item.Label + strings.Repeat(" ", padding) + " " + item.Detail
		}
		if len(displayText) > maxWidth {
			maxWidth = len(displayText)
		}
	}

	// Cap width to terminal width
	if maxWidth > w-10 {
		maxWidth = w - 10
	}

	popupWidth := maxWidth + 2
	popupHeight := len(e.autocompleteItems)
	if popupHeight > 10 {
		popupHeight = 10
	}

	// Calculate position (below cursor or above if no space)
	visualCursorX := e.bufferToVisual(b.buffer[b.PrimaryCursor().Y], b.PrimaryCursor().X)
	cursorScreenX := visualCursorX - b.scrollX + Config.GutterWidth
	cursorScreenY := b.PrimaryCursor().Y - b.scrollY

	startX := cursorScreenX
	startY := cursorScreenY + 1

	// Adjust if out of bounds
	if startY+popupHeight > h-1 {
		startY = cursorScreenY - popupHeight
	}
	if startX+popupWidth > w {
		startX = w - popupWidth
	}
	if startX < 0 {
		startX = 0
	}

	fg, bg := GetThemeColor(ColorAutocompleteWindow)
	selFg, selBg := GetThemeColor(ColorAutocompleteSelected)

	// Draw background and content
	for y := 0; y < popupHeight; y++ {
		itemIdx := y + e.autocompleteScroll
		if itemIdx >= len(e.autocompleteItems) {
			break
		}
		item := e.autocompleteItems[itemIdx]

		currentFg, currentBg := fg, bg
		if itemIdx == e.autocompleteIndex {
			currentFg, currentBg = selFg, selBg
		}

		// Fill line
		for x := 0; x < popupWidth; x++ {
			termbox.SetCell(startX+x, startY+y, ' ', currentFg, currentBg)
		}

		// Draw label and detail (signature) with alignment
		displayText := item.Label
		if item.Detail != "" {
			// Pad label to align with others
			padding := maxLabelWidth - len(item.Label)
			displayText = item.Label + strings.Repeat(" ", padding) + "  " + item.Detail
		}
		if len(displayText) > maxWidth {
			displayText = displayText[:maxWidth-3] + "..."
		}
		for j, r := range displayText {
			termbox.SetCell(startX+1+j, startY+y, r, currentFg, currentBg)
		}
	}
}

func (e *Editor) insertCompletion(item CompletionItem) {
	b := e.activeBuffer()
	if b == nil {
		return
	}

	cursor := b.PrimaryCursor()
	line := b.buffer[cursor.Y]

	// Find the start of the word we're completing
	start := cursor.X
	for start > 0 {
		r := line[start-1]
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			break
		}
		start--
	}

	// Text to insert
	insertText := item.InsertText
	if insertText == "" {
		insertText = item.Label
	}

	// Check if this is a function/method (Kind 2=Method, 3=Function)
	// or if the Detail contains "func" indicating it's a function
	isFunction := item.Kind == 2 || item.Kind == 3 || strings.Contains(item.Detail, "func")

	// Replace the prefix with the completion
	newRuneLine := make([]rune, start)
	copy(newRuneLine, line[:start])
	newRuneLine = append(newRuneLine, []rune(insertText)...)

	// Add () for functions if not already present
	cursorOffset := len(insertText)
	if isFunction {
		// Check if next character is already (
		nextIdx := cursor.X
		if nextIdx >= len(line) || line[nextIdx] != '(' {
			newRuneLine = append(newRuneLine, '(', ')')
			cursorOffset++ // Position cursor inside the parentheses
		}
	}

	newRuneLine = append(newRuneLine, line[cursor.X:]...)

	b.buffer[cursor.Y] = newRuneLine
	cursor.X = start + cursorOffset

	// Handle syntax update
	if b.syntax != nil {
		b.syntax.Reparse([]byte(b.toString()))
	}

	e.markModified()
	e.showAutocomplete = false
}
