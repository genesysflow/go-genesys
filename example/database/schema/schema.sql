-- Auto-generated schema dump

CREATE TABLE "create_user_table" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP
);

CREATE TABLE "users" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "name" VARCHAR(255) NOT NULL,
  "email" VARCHAR(255) NOT NULL UNIQUE,
  "password" VARCHAR(255),
  "birthdate" VARCHAR(255) NOT NULL,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP
);
