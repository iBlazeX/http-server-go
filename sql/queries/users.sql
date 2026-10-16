-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email)
VALUES (
    gen_random_uuid(), NOW(), NOW(), $1
)
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users;

-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid(), NOW(), NOW(), $1, $2
)RETURNING *;

-- name: ListAllChirps :many
select * FROM chirps order by created_at;

-- name: ListChirp :one
select * FROM chirps
where id = $1
order by created_at;
