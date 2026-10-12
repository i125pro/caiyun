package services

// QLBatchTasks 返回默认批量任务（单机/青龙脚本模式使用），按目录排序。
func QLBatchTasks() []TaskDefinition {
	out := []TaskDefinition{}
	for _, def := range defaultTaskCatalog.List() {
		if def.DefaultEnabled && def.RunInBatch {
			out = append(out, def)
		}
	}
	return out
}

// QLNormalizeTask 把别名转成任务编码，未知编码返回空。
func QLNormalizeTask(code string) string { return defaultTaskCatalog.Normalize(code) }

// QLTaskName 返回任务中文名。
func QLTaskName(code string) string {
	if def, ok := defaultTaskCatalog.Get(code); ok && def.Name != "" {
		return def.Name
	}
	return code
}

// QLJWTUserDomainID 从 JWT 中取 userDomainId，用于刷新 Authorization。
func QLJWTUserDomainID(token string) string { return jwtUserDomainID(token) }
