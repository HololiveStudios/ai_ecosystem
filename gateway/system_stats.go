package main

import (
	"fmt"
)

func GetSystemTelemetry() map[string]string {
	return map[string]string{
		"status": "online",
		"mode":   "local-gateway",
	}
}

func PrintTerminalDashboard() {
	fmt.Println("==================================================")
	fmt.Println("       ECOSSISTEMA INTELIGENTE LOCAL - GO         ")
	fmt.Println("==================================================")
}
