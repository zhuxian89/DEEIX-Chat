export function readSingleArgument(argv, usage, fallback) {
  const values = argv.filter((value) => value !== "--").map((value) => value.trim());

  if (values.length === 0 || values[0] === "") {
    if (fallback !== undefined && values.length === 0) {
      return fallback;
    }
    throw new Error(usage);
  }
  if (values.length !== 1) {
    throw new Error(`${usage}\nExpected exactly one argument.`);
  }

  return values[0];
}
