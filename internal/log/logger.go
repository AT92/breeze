package log

import (
	"fmt"
	"io"
	"os"
)

// Level controls the verbosity of log output.
type Level int

const (
	LevelQuiet   Level = iota // suppress all progress output
	LevelNormal               // standard progress output
	LevelVerbose              // include debug details
)

// Logger provides leveled output for CLI progress messages.
type Logger struct {
	output io.Writer
	level  Level
}

// New creates a Logger that writes to stderr at the given level.
func New(level Level) *Logger {
	return &Logger{
		output: os.Stderr,
		level:  level,
	}
}

// NewWithWriter creates a Logger that writes to the given writer at the given level.
func NewWithWriter(writer io.Writer, level Level) *Logger {
	return &Logger{
		output: writer,
		level:  level,
	}
}

// Info prints normal progress messages. Suppressed in quiet mode.
func (logger *Logger) Info(format string, args ...interface{}) {
	if logger.level < LevelNormal {
		return
	}
	fmt.Fprintf(logger.output, format+"\n", args...)
}

// Warn prints warning messages. Always shown unless quiet.
func (logger *Logger) Warn(format string, args ...interface{}) {
	if logger.level < LevelNormal {
		return
	}
	fmt.Fprintf(logger.output, "Warning: "+format+"\n", args...)
}

// Debug prints detailed messages. Only shown in verbose mode.
func (logger *Logger) Debug(format string, args ...interface{}) {
	if logger.level < LevelVerbose {
		return
	}
	fmt.Fprintf(logger.output, format+"\n", args...)
}

// Writer returns the underlying io.Writer, useful for passing to libraries
// that accept an io.Writer for their output. Returns io.Discard in quiet mode.
func (logger *Logger) Writer() io.Writer {
	if logger.level < LevelNormal {
		return io.Discard
	}
	return logger.output
}
