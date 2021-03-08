const prettier = require("prettier");
const { getFilePathsToFormat, getWorkspacePath } = require("./common");
const fs = require("fs");
const chalk = require("chalk");

/** Checks files which differ from the main branch for proper formatting. */
async function main() {
  const paths = await getFilePathsToFormat();
  const workspacePath = getWorkspacePath();
  const failedPaths = [];
  for (const path of paths) {
    const absolutePath = `${workspacePath}/${path}`;
    const config = await prettier.resolveConfig(absolutePath);
    const source = fs.readFileSync(absolutePath, { encoding: "utf-8" });
    process.stdout.write(`${path} `);
    if (
      !prettier.check(source, {
        ...config,
        filepath: path,
      })
    ) {
      process.stdout.write(chalk.red("FORMAT_ERRORS") + "\n");
      failedPaths.push(path);
    } else {
      process.stdout.write(chalk.green("OK") + "\n");
    }
  }

  if (failedPaths.length) {
    process.stderr.write(
      `\n${chalk.yellow(
        "Some files need formatting; to fix, run:\nbazel run //tools/prettier:format_modified_files"
      )}\n`
    );
    process.exit(1);
  }
}

main();
