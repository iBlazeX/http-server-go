package main

import (
	"MODULE_PATH/internal/auth"
	"MODULE_PATH/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConf struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	user           User
	platform       string
	jwt_secret     string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	User_ID   uuid.UUID `json:"user_id"`
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("Error fetching database")
	}
	jwt_secret := os.Getenv("JWT_SECRET")
	mux := http.NewServeMux()

	serv := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	apiCfg := apiConf{
		atomic.Int32{}, database.New(db), User{}, os.Getenv("PLATFORM"), jwt_secret,
	}

	fsv := http.FileServer(http.Dir("."))
	mux.HandleFunc("GET /api/healthz", health)
	handler := http.StripPrefix("/app", fsv)

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))

	mux.HandleFunc("GET /admin/metrics", apiCfg.metrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.reset)

	mux.HandleFunc("POST /api/login", apiCfg.login)
	mux.HandleFunc("POST /api/refresh", apiCfg.refreshToken)
	mux.HandleFunc("POST /api/revoke", apiCfg.revokeToken)
	mux.HandleFunc("POST /api/users", apiCfg.addUser)
	mux.HandleFunc("PUT /api/users", apiCfg.updateUser)
	mux.HandleFunc("POST /api/chirps", apiCfg.addChirp)
	mux.HandleFunc("GET /api/chirps", apiCfg.getAllChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)

	fmt.Printf("\033]8;;%s\033\\%s\033]8;;\033\\\n", "http://localhost:8080", "localhost:8080")
	serv.ListenAndServe()

}

func health(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	body := "OK"
	w.Write([]byte(body))
}

func respondWithError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("content-type", "application/json")
	payload := struct {
		Error string `json:"error"`
	}{
		Error: msg,
	}
	x, err := json.Marshal(payload)
	if err != nil {
		status = 500
	}
	w.WriteHeader(status)
	w.Write(x)
}

func respondWithJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json")
	x, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}
	w.WriteHeader(status)

	w.Write(x)
}

func profane(str string) string {
	arr := strings.Split(str, " ")
	for i := range arr {
		switch strings.ToLower(arr[i]) {
		case "kerfuffle", "sharbert", "fornax":
			arr[i] = "****"
		default:
			continue
		}
	}
	return strings.Join(arr, " ")
}

func (cfg *apiConf) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConf) metrics(w http.ResponseWriter, r *http.Request) {
	body := fmt.Sprintf(`<html>
		 <body>
		   <h1>Welcome, Chirpy Admin</h1>
		   <p>Chirpy has been visited %d times!</p>
		 </body>
	</html>`, cfg.fileserverHits.Load())
	w.Header().Set("content-type", "text/html")
	w.Write([]byte(body))
}

func (cfg *apiConf) reset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform == "dev" {
		err := cfg.dbQueries.DeleteUser(r.Context())
		if err != nil {
			respondWithError(w, 500, "Server Error")
		}
	} else {
		respondWithError(w, 403, "Forbidden")
	}
}

func (cfg *apiConf) login(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type response struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}

	decoder := json.NewDecoder(r.Body)
	var x request
	err := decoder.Decode(&x)
	if err != nil {
		respondWithError(w, 400, "Bad Request")
		return
	}

	data, err := cfg.dbQueries.ListUser(r.Context(), x.Email)
	pass, _ := auth.CheckPasswordHash(x.Password, data.HashedPassword)
	if err != nil || !pass {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	token, err := auth.MakeJWT(data.ID, cfg.jwt_secret, time.Hour)
	if err != nil {
		respondWithError(w, 400, "Server error")
		return
	}

	refresh_token := auth.MakeRefreshToken()

	_, err = cfg.dbQueries.CreateToken(r.Context(), database.CreateTokenParams{
		Token: refresh_token,
		UserID: uuid.NullUUID{
			UUID:  data.ID,
			Valid: true,
		},
		ExpiresAt: sql.NullTime{
			Time:  time.Now().Add(time.Hour * 24 * 60),
			Valid: true,
		},
	})
	if err != nil {
		respondWithError(w, 500, "Database Error")
	}

	res := response{
		data.ID, data.CreatedAt, data.UpdatedAt, data.Email, token, refresh_token,
	}

	respondWithJSON(w, 200, res)
}

func (cfg *apiConf) refreshToken(w http.ResponseWriter, r *http.Request) {
	refresh_token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}

	data, err := cfg.dbQueries.GetToken(r.Context(), refresh_token)
	if err != nil {
		respondWithError(w, 401, "Unauthorized Request")
		return
	}

	if data.ExpiresAt.Time.Before(time.Now()) || (data.RevokedAt.Valid) {
		respondWithError(w, 401, "Unauthorized Request")
		return
	}
	t, err := auth.MakeJWT(data.UserID.UUID, cfg.jwt_secret, time.Hour)
	if err != nil {
		respondWithError(w, 400, "Server error")
		return
	}
	respondWithJSON(w, 200, struct {
		Token string `json:"token"`
	}{
		Token: t,
	})
}

func (cfg *apiConf) revokeToken(w http.ResponseWriter, r *http.Request) {
	refresh_token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}
	err = cfg.dbQueries.RevokeToken(r.Context(), refresh_token)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}

	respondWithJSON(w, 204, nil)
}

func (cfg *apiConf) addUser(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	var x request
	err := decoder.Decode(&x)
	if err != nil {
		respondWithError(w, 400, "Bad Request")
		return
	}
	hash, err := auth.HashPassword(x.Password)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}
	u, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{Email: x.Email, HashedPassword: hash})
	if err != nil {
		respondWithError(w, 500, "Database Error")
		return
	}

	user := User{
		uuid.UUID(u.ID), u.CreatedAt, u.UpdatedAt, u.Email,
	}
	respondWithJSON(w, 201, user)
}

func (cfg *apiConf) updateUser(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	uid, err := auth.ValidateJWT(token, cfg.jwt_secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	var x request
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&x)
	if err != nil {
		respondWithError(w, 400, "Bad Request")
		return
	}

	hash, err := auth.HashPassword(x.Password)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}

	data, err := cfg.dbQueries.UpdateUser(r.Context(), database.UpdateUserParams{
		ID: uid, Email: x.Email, HashedPassword: hash,
	})
	if err != nil {
		respondWithError(w, 500, "Database Error")
		return
	}
	u := User{
		data.ID, data.CreatedAt, data.UpdatedAt, data.Email,
	}
	respondWithJSON(w, 200, u)
}

func (cfg *apiConf) addChirp(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Body    string    `json:"body"`
		User_ID uuid.UUID `json:"user_id"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}

	uid, err := auth.ValidateJWT(token, cfg.jwt_secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	decoder := json.NewDecoder(r.Body)
	var x request
	err = decoder.Decode(&x)

	if err != nil {
		respondWithError(w, 400, "Bad Request")
		return
	} else if len(x.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	} else {
		data, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{Body: x.Body, UserID: uuid.NullUUID{
			UUID:  uid,
			Valid: true,
		}})
		if err != nil {
			log.Printf("CreateChirp error: %v", err)
			respondWithError(w, 500, "Database Error")
			return
		}
		c := Chirp{
			data.ID, data.CreatedAt, data.UpdatedAt, data.Body, data.UserID.UUID,
		}
		respondWithJSON(w, 201, c)
	}
}

func (cfg *apiConf) getAllChirps(w http.ResponseWriter, r *http.Request) {
	data, err := cfg.dbQueries.ListAllChirps(r.Context())
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}
	chirps := make([]Chirp, 0, len(data))
	for _, c := range data {
		chirps = append(chirps, Chirp{
			ID:        uuid.UUID(c.ID),
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			User_ID:   c.UserID.UUID,
		})
	}
	respondWithJSON(w, 200, chirps)
}

func (cfg *apiConf) getChirp(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 400, "Bad Request")
		return
	}

	data, err := cfg.dbQueries.ListChirp(r.Context(), id)
	if err != nil {
		respondWithError(w, 404, "Chirp Not Found")
		return
	}
	c := Chirp{
		data.ID, data.CreatedAt, data.UpdatedAt, data.Body, data.UserID.UUID,
	}
	respondWithJSON(w, 200, c)
}
