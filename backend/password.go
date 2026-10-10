package main

import (
	"log"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(p string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		log.Println("hash_password failed:", err)
		return ""
	}
	return string(h)
}

func verifyPassword(p, h string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) == nil
}
