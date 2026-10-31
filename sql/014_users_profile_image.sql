-- 画像の取得で、プロフィール画像の持ち主を利用者の表から引く（webapp/go/image_location.go の servableImageByReference）。
-- 区画に分かれた image_objects に触らずに判定するため。索引が無いと、プロフィール画像の取得のたびに users を全件なめる。何度流しても壊れない
CREATE INDEX IF NOT EXISTS users_profile_image_id ON users(profile_image_id) WHERE profile_image_id IS NOT NULL;
