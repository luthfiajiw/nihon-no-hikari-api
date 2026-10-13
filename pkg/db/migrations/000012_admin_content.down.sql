BEGIN;
DROP TABLE public.admin_drafts;
DROP INDEX public.uq_question_set_version;
DROP INDEX public.uq_published_question_set_lineage;
DROP INDEX public.uq_published_legacy_final_exam_per_module;
DROP INDEX public.uq_published_final_exam_per_module_skill;
CREATE UNIQUE INDEX uq_final_exam_per_module ON public.question_sets(module_id)
  WHERE kind = 'final_exam';
CREATE UNIQUE INDEX uq_question_sets_lesson_order ON public.question_sets(lesson_id, order_index)
  WHERE lesson_id IS NOT NULL;
CREATE UNIQUE INDEX uq_question_sets_module_order ON public.question_sets(module_id, order_index)
  WHERE module_id IS NOT NULL;
ALTER TABLE public.question_sets DROP COLUMN version, DROP COLUMN lineage_id, DROP COLUMN archived_at;
ALTER TABLE public.lessons DROP COLUMN archived_at;
ALTER TABLE public.modules DROP COLUMN archived_at;
ALTER TABLE public.courses DROP COLUMN archived_at;
ALTER TABLE public.courses DROP COLUMN order_index;
ALTER TABLE public.enrollments DROP CONSTRAINT enrollments_course_id_fkey;
ALTER TABLE public.enrollments ADD CONSTRAINT enrollments_course_id_fkey
  FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE;
ALTER TABLE public.user_module_progress DROP CONSTRAINT user_module_progress_module_id_fkey;
ALTER TABLE public.user_module_progress ADD CONSTRAINT user_module_progress_module_id_fkey
  FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;
ALTER TABLE public.user_lesson_progress DROP CONSTRAINT user_lesson_progress_lesson_id_fkey;
ALTER TABLE public.user_lesson_progress ADD CONSTRAINT user_lesson_progress_lesson_id_fkey
  FOREIGN KEY (lesson_id) REFERENCES public.lessons(id) ON DELETE CASCADE;
ALTER TABLE public.attempts DROP CONSTRAINT attempts_question_set_id_fkey;
ALTER TABLE public.attempts ADD CONSTRAINT attempts_question_set_id_fkey
  FOREIGN KEY (question_set_id) REFERENCES public.question_sets(id) ON DELETE CASCADE;
ALTER TABLE public.attempt_answers DROP CONSTRAINT attempt_answers_question_id_fkey;
ALTER TABLE public.attempt_answers ADD CONSTRAINT attempt_answers_question_id_fkey
  FOREIGN KEY (question_id) REFERENCES public.questions(id) ON DELETE CASCADE;
COMMIT;
