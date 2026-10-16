package main

import (
	"MODULE_PATH/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("Error fetching database")
	}
	mux := http.NewServeMux()

	serv := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	apiCfg := apiConf{
		atomic.Int32{}, database.New(db), User{}, os.Getenv("PLATFORM"),
	}

	fsv := http.FileServer(http.Dir("."))
	mux.HandleFunc("GET /api/healthz", SrvHTTP)
	handler := http.StripPrefix("/app", fsv)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))
	mux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		body := fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
		</html>`, apiCfg.fileserverHits.Load())
		w.Header().Set("content-type", "text/html")
		w.Write([]byte(body))
	})
	mux.HandleFunc("POST /api/users", apiCfg.serveDB)
	mux.HandleFunc("POST /admin/reset", apiCfg.reset)
	mux.HandleFunc("POST /api/validate_chirp", validate)
	fmt.Printf("\033]8;;%s\033\\%s\033]8;;\033\\\n", "http://localhost:8080", "localhost:8080")
	serv.ListenAndServe()

}

func SrvHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	body := "OK"
	w.Write([]byte(body))
}

type apiConf struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	user           User
	platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cnf *apiConf) reset(w http.ResponseWriter, r *http.Request) {
	if cnf.platform == "dev" {
		err := cnf.dbQueries.DeleteUser(r.Context())
		if err != nil {
			respondWithError(w, 500, "Server Error")
		}
	} else {
		respondWithError(w, 403, "Forbidden")
	}
}

func (cnf *apiConf) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cnf.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, status int, msg string) {
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
	x, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}
	w.WriteHeader(status)

	w.Write(x)
}

func validate(w http.ResponseWriter, r *http.Request) {
	type param struct {
		Body string `json:"body"`
	}
	type resp struct {
		Cleaned_Body string `json:"cleaned_body"`
	}
	decoder := json.NewDecoder(r.Body)
	var x param
	err := decoder.Decode(&x)
	w.Header().Set("content-type", "application/json")
	if err != nil {
		respondWithError(w, 400, "Something went wrong")
	} else if len(x.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
	} else {
		res := resp{
			profane(x.Body),
		}
		respondWithJSON(w, 200, res)
	}

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

func (cfg *apiConf) serveDB(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	var x request
	err := decoder.Decode(&x)
	if err != nil {
		respondWithError(w, 400, "Bad Request")
		return
	}
	u, err := cfg.dbQueries.CreateUser(r.Context(), x.Email)
	if err != nil {
		respondWithError(w, 500, "Server Error")
		return
	}

	user := User{
		uuid.UUID(u.ID), u.CreatedAt, u.UpdatedAt, u.Email,
	}
	respondWithJSON(w, 201, user)
}
