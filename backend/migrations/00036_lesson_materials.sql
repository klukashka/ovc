-- +goose Up
ALTER TABLE lesson_series ADD CONSTRAINT lesson_series_id_tutor_key UNIQUE (id, tutor_id);
ALTER TABLE lessons ADD CONSTRAINT lessons_id_tutor_key UNIQUE (id, tutor_id);

ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_ref_kind_check;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_ref_kind_check CHECK (
    ref_kind IN ('message', 'answer', 'review_item', 'material', 'template', 'issue_snapshot', 'lesson', 'series', 'avatar')
);

CREATE TABLE lesson_materials (
    lesson_id uuid NOT NULL,
    material_id uuid NOT NULL,
    tutor_id uuid NOT NULL,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 19),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (lesson_id, material_id),
    CONSTRAINT lesson_materials_position_key UNIQUE (lesson_id, position),
    CONSTRAINT lesson_materials_lesson_fkey FOREIGN KEY (lesson_id, tutor_id) REFERENCES lessons (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT lesson_materials_material_fkey FOREIGN KEY (material_id, tutor_id) REFERENCES materials (id, tutor_id) ON DELETE RESTRICT
);

CREATE INDEX lesson_materials_material_idx ON lesson_materials (material_id);
CREATE INDEX lesson_materials_tutor_idx ON lesson_materials (tutor_id, lesson_id, material_id);

CREATE TABLE series_materials (
    series_id uuid NOT NULL,
    material_id uuid NOT NULL,
    tutor_id uuid NOT NULL,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 19),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (series_id, material_id),
    CONSTRAINT series_materials_position_key UNIQUE (series_id, position),
    CONSTRAINT series_materials_series_fkey FOREIGN KEY (series_id, tutor_id) REFERENCES lesson_series (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT series_materials_material_fkey FOREIGN KEY (material_id, tutor_id) REFERENCES materials (id, tutor_id) ON DELETE RESTRICT
);

CREATE INDEX series_materials_material_idx ON series_materials (material_id);
CREATE INDEX series_materials_tutor_idx ON series_materials (tutor_id, series_id, material_id);

-- +goose Down
DROP TABLE series_materials;
DROP TABLE lesson_materials;
DELETE FROM attachment_references WHERE ref_kind IN ('series', 'lesson');
ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_ref_kind_check;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_ref_kind_check CHECK (
    ref_kind IN ('message', 'answer', 'review_item', 'material', 'template', 'issue_snapshot', 'lesson', 'avatar')
);
ALTER TABLE lessons DROP CONSTRAINT lessons_id_tutor_key;
ALTER TABLE lesson_series DROP CONSTRAINT lesson_series_id_tutor_key;
