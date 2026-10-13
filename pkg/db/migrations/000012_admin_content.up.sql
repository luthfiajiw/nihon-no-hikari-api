BEGIN;

ALTER TABLE public.courses ADD COLUMN archived_at timestamptz;
ALTER TABLE public.courses ADD COLUMN order_index smallint NOT NULL DEFAULT 0 CHECK (order_index >= 0);
ALTER TABLE public.modules ADD COLUMN archived_at timestamptz;
ALTER TABLE public.lessons ADD COLUMN archived_at timestamptz;
ALTER TABLE public.question_sets ADD COLUMN archived_at timestamptz;
ALTER TABLE public.question_sets ADD COLUMN lineage_id uuid;
UPDATE public.question_sets SET lineage_id = id;
ALTER TABLE public.question_sets ALTER COLUMN lineage_id SET NOT NULL;
ALTER TABLE public.question_sets ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK (version > 0);

CREATE TABLE public.admin_drafts (
  resource_type text NOT NULL CHECK (resource_type IN ('course', 'module', 'lesson', 'question_set')),
  resource_id uuid NOT NULL,
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (resource_type, resource_id)
);

DROP INDEX public.uq_final_exam_per_module;
DROP INDEX public.uq_question_sets_lesson_order;
DROP INDEX public.uq_question_sets_module_order;
CREATE UNIQUE INDEX uq_published_final_exam_per_module_skill
  ON public.question_sets (module_id, skill)
  WHERE kind = 'final_exam' AND is_published AND archived_at IS NULL AND skill IS NOT NULL;
CREATE UNIQUE INDEX uq_published_legacy_final_exam_per_module
  ON public.question_sets (module_id)
  WHERE kind = 'final_exam' AND is_published AND archived_at IS NULL AND skill IS NULL;
CREATE UNIQUE INDEX uq_published_question_set_lineage
  ON public.question_sets (lineage_id)
  WHERE is_published AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_question_set_version ON public.question_sets (lineage_id, version);

-- History must never be removed by an administrative content deletion.
ALTER TABLE public.enrollments DROP CONSTRAINT enrollments_course_id_fkey;
ALTER TABLE public.enrollments ADD CONSTRAINT enrollments_course_id_fkey
  FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE RESTRICT;
ALTER TABLE public.user_module_progress DROP CONSTRAINT user_module_progress_module_id_fkey;
ALTER TABLE public.user_module_progress ADD CONSTRAINT user_module_progress_module_id_fkey
  FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE RESTRICT;
ALTER TABLE public.user_lesson_progress DROP CONSTRAINT user_lesson_progress_lesson_id_fkey;
ALTER TABLE public.user_lesson_progress ADD CONSTRAINT user_lesson_progress_lesson_id_fkey
  FOREIGN KEY (lesson_id) REFERENCES public.lessons(id) ON DELETE RESTRICT;
ALTER TABLE public.attempts DROP CONSTRAINT attempts_question_set_id_fkey;
ALTER TABLE public.attempts ADD CONSTRAINT attempts_question_set_id_fkey
  FOREIGN KEY (question_set_id) REFERENCES public.question_sets(id) ON DELETE RESTRICT;
ALTER TABLE public.attempt_answers DROP CONSTRAINT attempt_answers_question_id_fkey;
ALTER TABLE public.attempt_answers ADD CONSTRAINT attempt_answers_question_id_fkey
  FOREIGN KEY (question_id) REFERENCES public.questions(id) ON DELETE RESTRICT;

COMMIT;
