// Format the original tokens so large integers keep every digit.
export function formatJSON(source: string): string {
  try { JSON.parse(source); } catch { return source; }
  const tokens = source.match(/"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}\[\],:]/g) || [];
  let depth = 0;
  const output: string[] = [];
  const indent = () => "  ".repeat(depth);
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (token === "{" || token === "[") {
      output.push(token);
      if (tokens[i + 1] !== "}" && tokens[i + 1] !== "]") {
        depth++;
        output.push("\n", indent());
      }
    } else if (token === "}" || token === "]") {
      if (tokens[i - 1] !== "{" && tokens[i - 1] !== "[") {
        depth--;
        output.push("\n", indent());
      }
      output.push(token);
    } else if (token === ",") {
      output.push(",\n", indent());
    } else if (token === ":") {
      output.push(": ");
    } else {
      output.push(token);
    }
  }
  return output.join("");
}
