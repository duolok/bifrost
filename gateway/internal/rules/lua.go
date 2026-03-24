package rules

import (
	lua "github.com/yuin/gopher-lua"
)

// Creates a sandboxed Lua VM and sets up everything the script needs.
func newLuaState(event HealthEvent) *lua.LState {
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
	L.SetGlobal("_actions", actions)

	// Alert function is Go's function that is exposed to Lua.
	// - Reads the 3 arguments (severity, message, to)
	//    - Creates a Lua table with those values
	//    - Appends it to the _actions list
	L.SetGlobal("alert", L.NewFunction(func(L *lua.LState) int {
		severity := L.CheckString(1)
		message := L.CheckString(2)
		to := L.CheckString(3)

		action := L.NewTable()
		action.RawSetString("type", lua.LString("notify"))
		action.RawSetString("severity", lua.LString(severity))
		action.RawSetString("message", lua.LString(message))
		action.RawSetString("to", lua.LString(to))

		L.GetGlobal("_actions")
		tbl := L.ToTable(-1)
		tbl.Append(action)
		L.Pop(1)

		return 0
	}))

	return L
}

// Function that runs after Lua script finishes, converts lua table to GO action struct and returns a slice.
func collectActions(L *lua.LState) []Action {
	L.GetGlobal("_actions")
	tbl := L.ToTable(-1)
	if tbl == nil {
		return nil
	}

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
