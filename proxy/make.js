#!/usr/bin/env node

var fs = require('fs');
var { exec, spawn, execSync } = require('child_process');

// All files required.
var FILES = [
  'broker.js',
  'config.js',
  'proxypair.js',
  'snowflake.js',
  'ui.js',
  'util.js',
  'websocket.js',
  'shims.js'
];

var INITS = [
  'init-badge.js',
  'init-node.js',
  'init-webext.js'
];

var FILES_SPEC = [
  'spec/broker.spec.js',
  'spec/init.spec.js',
  'spec/proxypair.spec.js',
  'spec/snowflake.spec.js',
  'spec/ui.spec.js',
  'spec/util.spec.js',
  'spec/websocket.spec.js'
];

var OUTFILE = 'snowflake.js';

var STATIC = 'static';

var copyStaticFiles = function() {
  exec('cp ' + STATIC + '/* build/');
};

var concatJS = function(outDir, init) {
  var files;
  files = FILES.concat(`init-${init}.js`);
  return exec(`cat ${files.join(' ')} > ${outDir}/${OUTFILE}`, function(err, stdout, stderr) {
    if (err) {
      throw err;
    }
  });
};

var tasks = new Map();

var task = function(key, msg, func) {
  tasks.set(key, {
    msg, func
  });
};

task('test', 'snowflake unit tests', function() {
  var jasmineFiles, outFile, proc;
  exec('mkdir -p test');
  exec('jasmine init >&-');
  // Simply concat all the files because we're not using node exports.
  jasmineFiles = FILES.concat('init-badge.js', FILES_SPEC);
  outFile = 'test/bundle.spec.js';
  exec('echo "TESTING = true" > ' + outFile);
  exec('cat ' + jasmineFiles.join(' ') + ' | cat >> ' + outFile);
  proc = spawn('jasmine', ['test/bundle.spec.js'], {
    stdio: 'inherit'
  });
  proc.on("exit", function(code) {
    process.exit(code);
  });
});

task('build', 'build the snowflake proxy', function() {
  exec('mkdir -p build');
  copyStaticFiles();
  concatJS('build', 'badge');
  console.log('Snowflake prepared.');
});

task('webext', 'build the webextension', function() {
  exec('mkdir -p webext');
  concatJS('webext', 'webext');
  console.log('Webextension prepared.');
});

task('node', 'build the node binary', function() {
  exec('mkdir -p build');
  concatJS('build', 'node');
  console.log('Node prepared.');
});

task('clean', 'remove all built files', function() {
  exec('rm -r build test spec/support');
});

var cmd = process.argv[2];

if (tasks.has(cmd)) {
  var t = tasks.get(cmd);
  console.log(t.msg);
  t.func();
} else {
  console.error('Command not supported.');
}
