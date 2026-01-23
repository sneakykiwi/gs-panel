package forms

type Login struct {
	Email    string `form:"email"`
	Password string `form:"password"`
}

type Setup struct {
	Email           string `form:"email"`
	Password        string `form:"password"`
	ConfirmPassword string `form:"confirm_password"`
}

type CreateUser struct {
	Email    string `form:"email"`
	Password string `form:"password"`
	IsAdmin  string `form:"is_admin"`
}

type CreateServer struct {
	Name        string `form:"name"`
	GameType    string `form:"game_type"`
	MemoryLimit int    `form:"memory_limit"`
	Port        int    `form:"port"`
}

type SaveFile struct {
	Path    string `form:"path"`
	Content string `form:"content"`
}

type Upload struct {
	Path string `form:"path"`
}

type CreateDir struct {
	Path string `form:"path"`
	Name string `form:"name"`
}

type CreateBackup struct {
	Name        string `form:"name"`
	Description string `form:"description"`
}

type CreateSchedule struct {
	Name      string `form:"name"`
	CronExpr  string `form:"cron_expr"`
	KeepCount int    `form:"keep_count"`
	KeepDays  int    `form:"keep_days"`
}

type ToggleSchedule struct {
	Enabled string `form:"enabled"`
}

type AssignServer struct {
	ServerID string `form:"server_id"`
}
