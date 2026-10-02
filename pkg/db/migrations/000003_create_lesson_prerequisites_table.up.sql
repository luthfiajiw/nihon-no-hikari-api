CREATE TABLE "public"."lesson_prerequisites" (
  "lesson_id" uuid NOT NULL,
  "required_lesson_id" uuid NOT NULL,
  CONSTRAINT "lesson_prerequisites_check" CHECK ("lesson_id" <> "required_lesson_id"),
  CONSTRAINT "lesson_prerequisites_pkey" PRIMARY KEY ("lesson_id", "required_lesson_id"),
  CONSTRAINT "lesson_prerequisites_lesson_id_fkey"
    FOREIGN KEY ("lesson_id") REFERENCES "public"."lessons" ("id") ON DELETE CASCADE ON UPDATE NO ACTION,
  CONSTRAINT "lesson_prerequisites_required_lesson_id_fkey"
    FOREIGN KEY ("required_lesson_id") REFERENCES "public"."lessons" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

CREATE INDEX "idx_lesson_prereq_required"
  ON "public"."lesson_prerequisites" USING btree ("required_lesson_id");
