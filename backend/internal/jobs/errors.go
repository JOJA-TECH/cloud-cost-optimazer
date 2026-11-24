package jobs

import "errors"

var (
	// ErrJobNotFound devuelto cuando no existe un job con el id solicitado.
	ErrJobNotFound = errors.New("job not found")
	// ErrJobTerminal devuelto al intentar cancelar un job ya terminal.
	ErrJobTerminal = errors.New("job is already terminal")
	// ErrNoJobsAvailable devuelto por Claim cuando no hay jobs elegibles.
	ErrNoJobsAvailable = errors.New("no jobs available")
	// ErrNoHandler devuelto cuando no hay handler registrado para el tipo.
	ErrNoHandler = errors.New("no handler registered for job type")
	// ErrDependencyUnavailable devuelto cuando falta una dependencia requerida.
	ErrDependencyUnavailable = errors.New("dependency unavailable")
	// ErrInvalidInput devuelto por entrada inválida al encolar.
	ErrInvalidInput = errors.New("invalid input")
)
