// Command horoskopycli prints the horoskopy.cz horoscope for a zodiac sign.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kozaktomas/horoskopycli/internal/horoskopy"
)

// errTooManyArguments is returned when more than a sign and a period are given.
var errTooManyArguments = errors.New("too many arguments")

// Taken from https://github.com/NaAbAsD/this_is_fine, thanks!
const sorryMessage = `
Ouch, horoskopy.cz is probably down but I'm here for you! 🤗

     ..
    ...
     .    ..                .
      ..  _ .      .       ..
     .   |_| .  .. ..    .  .
    ..  -___-_. .   .. ..   ..
  ..   /      )      ..      .
 .____/| (0) (0)_()    ..     ..
/|   | |   ^____)      ..      ..
||   |_|    \_//     Uɔ....   .. ..
||    || |    |    ========.  ..  ..
||    || |    |      ||     ..   .
||     \\_\   |\     ||   ...    .
=========||====||    ||  ..       .
  || ||   \Ɔ || \Ɔ   ||   ..    ..
  || ||      ||      ||  .     ..
-------------------------------------
            This is fine.
`

// fetcher downloads a horoscope. It is satisfied by horoskopy.Client and
// replaced by a fake in tests.
type fetcher interface {
	Fetch(ctx context.Context, sign horoskopy.Sign, period horoskopy.Period) (horoskopy.Horoscope, error)
}

// main wires up the real client and turns a failure into a non-zero exit code.
func main() {
	if err := run(context.Background(), horoskopy.NewClient(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "horoskopycli: %s\n", err)
		os.Exit(1)
	}
}

// run parses args, fetches the requested horoscope and writes it to out.
//
// Called without arguments it prints usage and succeeds, so that a bare
// invocation is not an error. Invalid input returns an error describing the
// problem; an unreachable horoskopy.cz additionally writes an apology to out.
func run(ctx context.Context, client fetcher, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage())

		return nil
	}

	sign, period, err := parseArgs(args)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, usage())
	}

	horoscope, err := client.Fetch(ctx, sign, period)
	if err != nil {
		if errors.Is(err, horoskopy.ErrUnavailable) {
			fmt.Fprint(out, sorryMessage)
		}

		return err
	}

	fmt.Fprint(out, horoskopy.Render(horoscope))

	return nil
}

// parseArgs reads the zodiac sign and the optional period from the command
// line. The period defaults to today when omitted.
func parseArgs(args []string) (horoskopy.Sign, horoskopy.Period, error) {
	if len(args) > 2 {
		return "", "", fmt.Errorf("%w: expected a sign and an optional period, got %d", errTooManyArguments, len(args))
	}

	sign, err := horoskopy.ParseSign(args[0])
	if err != nil {
		return "", "", err
	}

	if len(args) == 1 {
		return sign, horoskopy.DefaultPeriod, nil
	}

	period, err := horoskopy.ParsePeriod(args[1])
	if err != nil {
		return "", "", err
	}

	return sign, period, nil
}

// usage returns the help text listing the accepted signs and periods.
func usage() string {
	return fmt.Sprintf(`Usage: horoskopycli <sign> [period]

Signs:   %s
Periods: %s (default: %s)

Examples:
  horoskopycli byk
  horoskopycli ryby zitra
  horoskopycli lev rok
`,
		strings.Join(horoskopy.SignSlugs(), ", "),
		strings.Join(horoskopy.PeriodSlugs(), ", "),
		horoskopy.DefaultPeriod,
	)
}
