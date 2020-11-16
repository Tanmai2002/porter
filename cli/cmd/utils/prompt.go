package utils

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

// PromptPlaintext prompts a user to input plain text
func PromptPlaintext(prompt string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print(prompt)
	text, err := reader.ReadString('\n')

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(text), nil
}

// PromptPassword prompts a user to input a hidden field
func PromptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(0)
	fmt.Print("\r")

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(pw)), nil
}

// PromptPasswordWithConfirmation is a helper function to prompt
// for a password twice
func PromptPasswordWithConfirmation() (string, error) {
	pw, err := PromptPassword("Password: ")
	if err != nil {
		return "", err
	}
	confirmPw, err := PromptPassword("Confirm password: ")
	if err != nil {
		return "", err
	}

	if pw != confirmPw {
		return "", errors.New("Passwords do not match")
	}

	return pw, nil
}
