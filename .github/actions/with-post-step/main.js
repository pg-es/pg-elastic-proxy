const {spawn} = require("child_process");
const {appendFileSync} = require("fs");
const {EOL} = require("os");

function run(cmd) {
  const shell = process.env.INPUT_SHELL || 'bash';

  const subprocess = spawn(cmd, {
    stdio: "inherit",
    shell: shell
  });
  subprocess.on("exit", (exitCode) => {
    process.exitCode = exitCode;
  });
}

const key = process.env.INPUT_KEY.toUpperCase();

if (process.env[`STATE_${key}`] !== undefined) { // Are we in the 'post' step?
  console.log(`Executing post step (${key})`);
  run(process.env.INPUT_POST);
} else { // Otherwise, this is the main step
  console.log(`Executing main step (${key})`);
  appendFileSync(process.env.GITHUB_STATE, `${key}=true${EOL}`);
  run(process.env.INPUT_MAIN);
}
