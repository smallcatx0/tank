CREATE TABLE "users" (
  "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  "user" TEXT NOT NULL,
  "pass" TEXT NOT NULL,
  "nickname" TEXT NOT NULL,
  "truename" TEXT,
  "phone" TEXT,
  "email" TEXT,
  "status" integer,
  "created_at" integer,
  "updated_at" integer
);
