-- +goose Up
CREATE TABLE tokens(
	token string primary key,
	created_at timestamp not null,
	updated_at timestamp not null,
	user_id uuid,
	expires_at timestamp,
	revoked_at timestamp,
constraint fk_user
	foreign key (user_id) references users(id)
	on delete cascade);

-- +goose Down
DROP TABLE tokens;
