package linkicon

import "testing"

func TestFind(t *testing.T) {
	testCases := []struct {
		baseRawURL string
		body       string
		expected   string
	}{
		{
			"https://a/stream",
			`
			<link href="https://a/favicon.ico" rel="icon">
			<link href="https://a/fluid.png" rel="fluid-icon">
			<link href="https://a/ios.png" rel="apple-touch-icon">
			`,
			"https://a/fluid.png",
		},
		{
			"https://a/stream",
			`
			<link rel="apple-touch-icon" sizes="180x180" href="https://a/180.png">
			<link rel="apple-touch-icon" sizes="57x57" href="https://a/57.png">
			<link rel="icon" type="image/png" href="https://a/192.png" sizes="192x192">
			`,
			"https://a/192.png",
		},
		{
			"https://a/stream",
			`
			<link rel="icon" href="/favicon-48.png" sizes="48x48" >
			<link rel="icon" href="/favicon-144.png" sizes="144x144" >
			`,
			"https://a/favicon-144.png",
		},
	}
	for tCIndex, tC := range testCases {
		actual, actualErr := Find(tC.baseRawURL, tC.body)
		if actualErr != nil {
			t.Errorf("%d: got error %v", tCIndex, actualErr)
		}
		if actual != tC.expected {
			t.Errorf("%d: expected %s got %s", tCIndex, tC.expected, actual)
		}
	}
}
