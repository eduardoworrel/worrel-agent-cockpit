package store

import "testing"

func TestEngineEnabledDefault(t *testing.T) {
	st, _ := Open(t.TempDir() + "/t.db")
	defer st.Close()
	if st.EngineEnabled("summary", "s1", false) != false {
		t.Fatal("sem config, deve cair no default false")
	}
	if st.EngineEnabled("interpret", "", true) != true {
		t.Fatal("sem config, deve cair no default true")
	}
}

func TestEngineEnabledGlobalOverride(t *testing.T) {
	st, _ := Open(t.TempDir() + "/t.db")
	defer st.Close()
	if err := st.SetEngineConfig("summary", "__enabled", "true", ""); err != nil {
		t.Fatal(err)
	}
	if st.EngineEnabled("summary", "s1", false) != true {
		t.Fatal("global true deve sobrepor o default false")
	}
}

func TestEngineEnabledSessionBeatsGlobal(t *testing.T) {
	st, _ := Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "__enabled", "true", "")             // global ON
	_ = st.SetEngineConfig("summary", "__enabled", "false", "session:s1") // sessão OFF
	if st.EngineEnabled("summary", "s1", false) != false {
		t.Fatal("override de sessão deve vencer o global")
	}
	if st.EngineEnabled("summary", "s2", false) != true {
		t.Fatal("outra sessão sem override segue o global ON")
	}
}
