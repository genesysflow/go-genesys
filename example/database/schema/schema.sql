-- Auto-generated schema dump

CREATE TABLE "attachments" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "attachable_type" VARCHAR(100) NOT NULL,
  "attachable_id" INTEGER NOT NULL,
  "disk" VARCHAR(50) NOT NULL,
  "path" VARCHAR(255) NOT NULL,
  "size" INTEGER NOT NULL,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP
);

CREATE INDEX "attachments_attachable_type_attachable_id_index" ON "attachments" ("attachable_type", "attachable_id");

CREATE TABLE "comments" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "post_id" INTEGER NOT NULL,
  "author_id" INTEGER NOT NULL,
  "body" TEXT NOT NULL,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP
);

CREATE INDEX "comments_post_id_index" ON "comments" ("post_id");

CREATE TABLE "create_user_table" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP
);

CREATE TABLE "failed_jobs" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "queue" VARCHAR(255) NOT NULL,
  "name" VARCHAR(255) NOT NULL,
  "payload" TEXT NOT NULL,
  "exception" TEXT NOT NULL,
  "failed_at" INTEGER NOT NULL
);

CREATE TABLE "jobs" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "queue" VARCHAR(255) NOT NULL,
  "name" VARCHAR(255) NOT NULL,
  "payload" TEXT NOT NULL,
  "attempts" INTEGER NOT NULL,
  "reserved_at" INTEGER,
  "available_at" INTEGER NOT NULL,
  "created_at" INTEGER NOT NULL
);

CREATE INDEX "jobs_queue_index" ON "jobs" ("queue");

CREATE TABLE "notifications" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "notifiable_id" VARCHAR(100) NOT NULL,
  "name" VARCHAR(255) NOT NULL,
  "data" TEXT NOT NULL,
  "read_at" TIMESTAMP,
  "created_at" TIMESTAMP
);

CREATE INDEX "notifications_notifiable_id_index" ON "notifications" ("notifiable_id");

CREATE TABLE "password_reset_tokens" (
  "email" VARCHAR(255) NOT NULL UNIQUE,
  "token" VARCHAR(64) NOT NULL,
  "created_at" TIMESTAMP
);

CREATE TABLE "personal_access_tokens" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "tokenable_type" VARCHAR(255) NOT NULL,
  "tokenable_id" INTEGER NOT NULL,
  "name" VARCHAR(255) NOT NULL,
  "token" VARCHAR(64) NOT NULL UNIQUE,
  "abilities" TEXT,
  "last_used_at" TIMESTAMP,
  "expires_at" TIMESTAMP,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP
);

CREATE INDEX "personal_access_tokens_tokenable_type_tokenable_id_index" ON "personal_access_tokens" ("tokenable_type", "tokenable_id");

CREATE TABLE "posts" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "author_id" INTEGER NOT NULL,
  "title" VARCHAR(255) NOT NULL,
  "slug" VARCHAR(255) NOT NULL UNIQUE,
  "body" TEXT NOT NULL,
  "published_at" TIMESTAMP,
  "created_at" TIMESTAMP,
  "updated_at" TIMESTAMP,
  "deleted_at" TIMESTAMP
);

CREATE INDEX "posts_author_id_index" ON "posts" ("author_id");

CREATE TABLE "taggables" (
  "tag_id" INTEGER NOT NULL,
  "taggable_id" INTEGER NOT NULL,
  "taggable_type" VARCHAR(100) NOT NULL
);

CREATE INDEX "taggables_taggable_type_taggable_id_index" ON "taggables" ("taggable_type", "taggable_id");

CREATE TABLE "tags" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "name" VARCHAR(100) NOT NULL,
  "slug" VARCHAR(100) NOT NULL UNIQUE,
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
, "role" VARCHAR(20) NOT NULL DEFAULT 'author');
