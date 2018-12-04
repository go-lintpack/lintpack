package checker_test

func nilPanics() {
	_ = func() {
		/*! panic(nil) calls are discouraged */
		panic(nil)
	}

	_ = func() {
		/*! panic(interface{}(nil)) calls are discouraged */
		panic(interface{}(nil))
	}
}
