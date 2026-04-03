// Package iostreams provides an abstraction over standard I/O with TTY awareness,
// color control, and test substitution. Commands use IOStreams instead of os.Stdin,
// os.Stdout, and os.Stderr directly, enabling testable output and consistent
// terminal behavior across the application.
package iostreams
