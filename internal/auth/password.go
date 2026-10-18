package auth

import "github.com/alexedwards/argon2id"

func HashPassword(password string) (string, error) {
	param := argon2id.DefaultParams
	return argon2id.CreateHash(password, param)

}
func CheckPasswordHash(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}
