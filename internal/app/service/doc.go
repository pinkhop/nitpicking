// Package service defines the application service layer — the driving port
// that CLI and other adapters invoke. Each service method orchestrates domain
// logic and persistence calls within transactions. Input and output are
// defined as DTOs; errors use domain error types.
package service
