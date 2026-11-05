package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"

	"golang.org/x/term"
)

var errOperatorRejected = errors.New("operator credentials rejected")

// stdin is shared across prompts: a fresh bufio.Reader per call would discard
// whatever the previous one had already buffered.
var stdin = bufio.NewReader(os.Stdin)

// promptLine reads one visible line from the terminal.
func promptLine(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	line, err := stdin.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptSecret reads a line without echo. Passwords are never taken from flags
// or arguments, which would land in shell history and the process list.
func promptSecret(label string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("stdin is not a terminal; run this command interactively")
	}
	fmt.Fprint(os.Stderr, label)
	secret, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

// authenticateOperator asks who is running the command and proves it with their
// own password. Only an active superadmin passes; every failure returns the same
// error so the prompt does not reveal which usernames exist.
func authenticateOperator(ctx context.Context, users domain.UserRepository) error {
	username, err := promptLine("Operator username: ")
	if err != nil {
		return err
	}
	password, err := promptSecret("Operator password: ")
	if err != nil {
		return err
	}
	operator, err := users.GetUserByUsername(ctx, username)
	if err != nil {
		return err
	}
	if operator == nil || operator.Role != enum.Superadmin || !operator.ComparedPwd(password) {
		return errOperatorRejected
	}
	return nil
}
