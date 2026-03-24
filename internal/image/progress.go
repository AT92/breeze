package image

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// trackingWriter wraps an io.Writer and reports bytes written to the display.
type trackingWriter struct {
	destination    io.Writer
	slotIndex      int
	totalBytes     int64
	bytesWritten   atomic.Int64
	display        *progressDisplay
}

func (writer *trackingWriter) Write(data []byte) (int, error) {
	bytesWritten, err := writer.destination.Write(data)
	if bytesWritten > 0 {
		currentBytes := writer.bytesWritten.Add(int64(bytesWritten))
		writer.display.updateSlotProgress(writer.slotIndex, currentBytes, writer.totalBytes)
	}
	return bytesWritten, err
}

// --- Progress Display ---
// Manages N fixed lines on the terminal, one per concurrent download slot.
// A background ticker redraws the lines periodically using ANSI escape codes.

type slotState struct {
	active       bool
	reference    string
	status       string
	currentBytes int64
	totalBytes   int64
}

type progressDisplay struct {
	slots           []slotState
	mutex           sync.Mutex
	output          io.Writer
	printedLines    int
	completedMessages []string
	done            chan struct{}
}

func newProgressDisplay(concurrency int) *progressDisplay {
	return &progressDisplay{
		slots:  make([]slotState, concurrency),
		output: os.Stderr,
		done:   make(chan struct{}),
	}
}

func (display *progressDisplay) start() {
	display.mutex.Lock()
	for index := range display.slots {
		fmt.Fprintf(display.output, "  [%d] idle\n", index+1)
	}
	display.printedLines = len(display.slots)
	display.mutex.Unlock()

	go display.runRedrawLoop()
}

func (display *progressDisplay) runRedrawLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-display.done:
			return
		case <-ticker.C:
			display.redraw()
		}
	}
}

func (display *progressDisplay) stop() {
	close(display.done)
	display.mutex.Lock()
	defer display.mutex.Unlock()

	display.clearSlotLines()
	display.printCompletedMessages()
}

func (display *progressDisplay) clearSlotLines() {
	if display.printedLines > 0 {
		fmt.Fprintf(display.output, "\033[%dA", display.printedLines)
	}
	for range display.slots {
		fmt.Fprintf(display.output, "\033[2K\n")
	}
	if len(display.slots) > 0 {
		fmt.Fprintf(display.output, "\033[%dA", len(display.slots))
	}
}

func (display *progressDisplay) printCompletedMessages() {
	for _, message := range display.completedMessages {
		fmt.Fprintln(display.output, message)
	}
}

func (display *progressDisplay) allocSlot(reference string) int {
	display.mutex.Lock()
	defer display.mutex.Unlock()
	for index := range display.slots {
		if !display.slots[index].active {
			display.slots[index] = slotState{active: true, reference: reference, status: "starting..."}
			return index
		}
	}
	return 0
}

func (display *progressDisplay) freeSlot(slotIndex int) {
	display.mutex.Lock()
	defer display.mutex.Unlock()
	display.slots[slotIndex] = slotState{}
}

func (display *progressDisplay) updateSlot(slotIndex int, reference string, status string, currentBytes, totalBytes int64) {
	display.mutex.Lock()
	defer display.mutex.Unlock()
	display.slots[slotIndex].reference = reference
	display.slots[slotIndex].status = status
	display.slots[slotIndex].currentBytes = currentBytes
	display.slots[slotIndex].totalBytes = totalBytes
}

func (display *progressDisplay) updateSlotProgress(slotIndex int, currentBytes, totalBytes int64) {
	display.mutex.Lock()
	defer display.mutex.Unlock()
	display.slots[slotIndex].currentBytes = currentBytes
	display.slots[slotIndex].totalBytes = totalBytes
	display.slots[slotIndex].status = "saving..."
}

func (display *progressDisplay) logMessage(format string, args ...interface{}) {
	display.mutex.Lock()
	defer display.mutex.Unlock()
	display.completedMessages = append(display.completedMessages, fmt.Sprintf(format, args...))
}

func (display *progressDisplay) redraw() {
	display.mutex.Lock()
	defer display.mutex.Unlock()

	if display.printedLines == 0 {
		return
	}

	fmt.Fprintf(display.output, "\033[%dA", display.printedLines)

	for _, slot := range display.slots {
		fmt.Fprintf(display.output, "\033[2K")
		if !slot.active {
			fmt.Fprintf(display.output, "  --\n")
		} else if slot.totalBytes > 0 && slot.status == "saving..." {
			display.renderProgressLine(slot)
		} else {
			fmt.Fprintf(display.output, "  %s %s\n", truncateReference(slot.reference), slot.status)
		}
	}
}

func (display *progressDisplay) renderProgressLine(slot slotState) {
	percentage := float64(slot.currentBytes) / float64(slot.totalBytes) * 100
	if percentage > 100 {
		percentage = 100
	}
	bar := renderProgressBar(percentage, 20)
	fmt.Fprintf(display.output, "  %s %s %s / %s (%.0f%%)\n",
		truncateReference(slot.reference), bar,
		formatBytes(slot.currentBytes), formatBytes(slot.totalBytes), percentage)
}

// truncateReference shortens an image reference to fit the display width.
func truncateReference(reference string) string {
	const maxLength = 50
	if len(reference) <= maxLength {
		return fmt.Sprintf("%-*s", maxLength, reference)
	}
	return "..." + reference[len(reference)-maxLength+3:]
}

// renderProgressBar generates a text progress bar like [========>       ].
func renderProgressBar(percentage float64, width int) string {
	filledCount := int(percentage / 100 * float64(width))
	if filledCount > width {
		filledCount = width
	}
	bar := strings.Repeat("=", filledCount)
	if filledCount < width {
		bar += ">"
		bar += strings.Repeat(" ", width-filledCount-1)
	}
	return "[" + bar + "]"
}
