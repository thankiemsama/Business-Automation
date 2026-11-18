package main

import (
	"businessautomation/internal/service"
	"flag"
	"fmt"
	"os"
)

func main() {
	tenant := flag.String("tenant", "", "Tenant ID")
	input := flag.String("input", "", "Encrypted backup")
	output := flag.String("output", "", "New JSON output")
	flag.Parse()
	if err := unseal(*tenant, *input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func unseal(tenant, input, output string) error {
	if tenant == "" || input == "" || output == "" {
		return fmt.Errorf("required: -tenant, -input, -output")
	}
	sealed, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	payload, err := service.OpenTenantBackup(tenant, string(sealed))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(payload)
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}
