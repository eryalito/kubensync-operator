# Template Functions

The template engine used by kubensync is based on the Go template engine. It contains the default functions provided by the Go template engine, as well as some additional functions.

## Default Functions

The default functions provided by the Go template engine are available in the template context. A highlight of the most commonly used functions is provided below. For a complete list of functions, see the [Go template documentation](https://golang.org/pkg/text/template/).

- `len`: Returns the length of a string, array, slice, or map.
- `index`: Returns the element at the specified index in an array, slice, or map.
- `slice`: Returns a slice of the specified array or slice.
- `printf`: Formats a string using the specified format and arguments.

## Additional Functions

Both [Sprig](https://github.com/Masterminds/sprig) and [Sprout](https://github.com/go-sprout/sprout) are included in the template engine. These libraries provide a set of additional functions that can be used to manipulate strings, arrays, maps and objects.

Highlighted functions include:

- `base64Encode`: Encodes a string in base64.
- `base64Decode`: Decodes a base64 encoded string.
- `fromYAML`: Converts a YAML string to a map.
- `toYAML`: Converts a map to a YAML string.
- `trim`: Trims whitespace from a string.
- `join`: Joins a list of strings into a single string using the specified separator.
- `split`: Splits a string into a list of strings using the specified separator.
- `indent`: Indents a string by the specified number of spaces.

!!! info
    Full list of functions can be found in the [Sprig documentation](https://masterminds.github.io/sprig) and [Sprout documentation](https://docs.atom.codes/sprout/registries/list-of-all-registries)
