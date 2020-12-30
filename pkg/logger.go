package pkg

import "fmt"

type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
	Info(v ...interface{})
	Infof(format string, v ...interface{})
	Warn(v ...interface{})
	Warnf(format string, v ...interface{})
	Error(v ...interface{})
	Errorf(format string, v ...interface{})
}

type DummyLogger struct {
}

func (log *DummyLogger) Debug(_ ...interface{}) {

}

func (log *DummyLogger) Debugf(_ string, _ ...interface{}) {

}

func (log *DummyLogger) Info(_ ...interface{}) {

}

func (log *DummyLogger) Infof(_ string, _ ...interface{}) {

}

func (log *DummyLogger) Warn(_ ...interface{}) {

}

func (log *DummyLogger) Warnf(_ string, _ ...interface{}) {

}

func (log *DummyLogger) Error(_ ...interface{}) {

}

func (log *DummyLogger) Errorf(_ string, _ ...interface{}) {

}

type ConsoleLogger struct {
}

func (log *ConsoleLogger) Debug(v ...interface{}) {
	fmt.Println(v...)
}

func (log *ConsoleLogger) Debugf(format string, v ...interface{}) {
	fmt.Printf(format+"\n", v...)
}

func (log *ConsoleLogger) Info(v ...interface{}) {
	fmt.Println(v...)
}

func (log *ConsoleLogger) Infof(format string, v ...interface{}) {
	fmt.Printf(format+"\n", v...)
}

func (log *ConsoleLogger) Warn(v ...interface{}) {
	fmt.Println(v...)
}

func (log *ConsoleLogger) Warnf(format string, v ...interface{}) {
	fmt.Printf(format+"\n", v...)
}

func (log *ConsoleLogger) Error(v ...interface{}) {
	fmt.Println(v...)
}

func (log *ConsoleLogger) Errorf(format string, v ...interface{}) {
	fmt.Printf(format+"\n", v...)
}
