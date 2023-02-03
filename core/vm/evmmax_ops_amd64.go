package vm

//go:noescape
func opAddModX(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error)
