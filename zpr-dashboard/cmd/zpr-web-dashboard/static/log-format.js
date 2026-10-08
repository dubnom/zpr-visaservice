(() => {
  function formatJSON(text) {
    const source = text.trim();
    if (!source.startsWith("{") && !source.startsWith("[")) return text;
    try {
      JSON.parse(source);
    } catch (error) {
      if (error instanceof SyntaxError) return text;
      throw error;
    }
    // Format tokens, not parsed values, to retain integer precision and duplicate keys.
    const tokens = source.match(/"(?:\\.|[^"\\])*"|[^\s]/g);
    let output = "";
    let depth = 0;
    const newline = () => { output += `\n${"  ".repeat(depth)}`; };
    for (let index = 0; index < tokens.length; index++) {
      const token = tokens[index];
      if (token === "{" || token === "[") {
        output += token;
        depth++;
        if (depth > 64) return text;
        if (tokens[index + 1] !== "}" && tokens[index + 1] !== "]") newline();
      } else if (token === "}" || token === "]") {
        depth--;
        if (tokens[index - 1] !== "{" && tokens[index - 1] !== "[") newline();
        output += token;
      } else if (token === ",") {
        output += token;
        newline();
      } else {
        output += token === ":" ? ": " : token;
      }
    }
    return output;
  }

  window.ZPRLogFormat = Object.freeze({ formatJSON });
})();
