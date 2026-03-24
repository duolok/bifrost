package rules

import (
	lua "github.com/yuin/gopher-lua"
)

// newLuaState creates a sandboxed Lua VM. Returns the VM and the actions table.
func newLuaState(event HealthEvent) (*lua.LState, *lua.LTable) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})

	lua.OpenBase(L)
	lua.OpenMath(L)
	lua.OpenString(L)
	lua.OpenTable(L)

	L.SetGlobal("healthy", lua.LBool(event.Healthy))
	L.SetGlobal("response_time_ms", lua.LNumber(event.ResponseTimeMs))
	L.SetGlobal("cpu_percent", lua.LNumber(event.CpuPercent))
	L.SetGlobal("memory_percent", lua.LNumber(event.MemoryPercent))
	L.SetGlobal("unhealthy_count", lua.LNumber(event.UnhealthyCount))
	L.SetGlobal("deploy_id", lua.LString(event.DeployID))
	L.SetGlobal("project", lua.LString(event.Project))

	actions := L.NewTable()

	// alert(severity, message, to) — captures actions table directly
	L.SetGlobal("alert", L.NewFunction(func(L *lua.LState) int {
		severity := L.CheckString(1)
		message := L.CheckString(2)
		to := L.CheckString(3)

		action := L.NewTable()
		action.RawSetString("type", lua.LString("notify"))
		action.RawSetString("severity", lua.LString(severity))
		action.RawSetString("message", lua.LString(message))
		action.RawSetString("to", lua.LString(to))

		actions.Append(action)
		return 0
	}))

	return L, actions
}

// collectActions converts the Lua actions table to Go Action structs.
func collectActions(tbl *lua.LTable) []Action {
	var actions []Action
	tbl.ForEach(func(_, v lua.LValue) {
		if t, ok := v.(*lua.LTable); ok {
			actions = append(actions, Action{
				Type:     t.RawGetString("type").String(),
				Severity: t.RawGetString("severity").String(),
				Message:  t.RawGetString("message").String(),
				To:       t.RawGetString("to").String(),
			})
		}
	})
	return actions
}
