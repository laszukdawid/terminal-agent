package voice

type Trigger interface {
	Register(onToggle func())
}
