package checker_test

func goodPanics() {
	_ = func() {
		panic("unreachable")
	}

	_ = func() {
		panic(404)
	}
}
