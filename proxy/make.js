#!/usr/bin/env node

/* global require, process */

var { writeFileSync, readdirSync, statSync } = require('fs');
var { execSync, spawn } = require('child_process');
var cldr = require('cldr');

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

var FILES_SPEC = [
  'spec/broker.spec.js',
  'spec/init.spec.js',
  'spec/proxypair.spec.js',
  'spec/snowflake.spec.js',
  'spec/ui.spec.js',
  'spec/util.spec.js',
  'spec/websocket.spec.js'
];

var STATIC = 'static';

var SHARED_FILES = [
  'embed.html',
  'embed.css',
  'popup.js',
  'assets',
  '_locales',
];

var concatJS = function(outDir, init, outFile, pre) {
  var files = FILES;
  if (init) {
    files = files.concat(`init-${init}.js`);
  }
  var outPath = `${outDir}/${outFile}`;
  writeFileSync(outPath, pre, 'utf8');
  execSync(`cat ${files.join(' ')} >> ${outPath}`);
};

var copyTranslations = function(outDir) {
  execSync('git submodule update --init -- translation')
  execSync(`cp -rf translation/* ${outDir}/_locales/`);
};

var getDisplayName = function(locale) {
  var code = locale.split("_")[0];
  try {
    var name = cldr.extractLanguageDisplayNames(code)[code];
  }
  catch(e) {
    return '';
  }
  if (name === undefined) {
    return '';
  }
  return name;
}

var availableLangs = function() {
  let out = "const availableLangs = new Set([\n";
  let dirs = readdirSync('translation').filter((f) => {
    const s = statSync(`translation/${f}`);
    return s.isDirectory();
  });
  dirs.push('en_US');
  dirs.sort();
  dirs = dirs.map(d => `  '${d}',`);
  out += dirs.join("\n");
  out += "\n]);\n\n";
  return out;
};

var translatedLangs = function() {
  let out = "const availableLangs = {\n";
  let dirs = readdirSync('translation').filter((f) => {
    const s = statSync(`translation/${f}`);
    return s.isDirectory();
  });
  dirs.push('en_US');
  dirs.sort();
  dirs = dirs.map(d => `'${d}': {"name": '${getDisplayName(d)}'},`);
  out += dirs.join("\n");
  out += "\n};\n\n";
  return out;
};
var tasks = new Map();

var task = function(key, msg, func) {
  tasks.set(key, {
    msg, func
  });
};

task('test', 'snowflake unit tests', function() {
  var jasmineFiles, outFile, proc;
  execSync('mkdir -p test');
  execSync('jasmine init >&-');
  // Simply concat all the files because we're not using node exports.
  jasmineFiles = FILES.concat('init-testing.js', FILES_SPEC);
  outFile = 'test/bundle.spec.js';
  execSync('echo "TESTING = true" > ' + outFile);
  execSync('cat ' + jasmineFiles.join(' ') + ' | cat >> ' + outFile);
  proc = spawn('jasmine', ['test/bundle.spec.js'], {
    stdio: 'inherit'
  });
  proc.on("exit", function(code) {
    process.exit(code);
  });
});

task('build', 'build the snowflake proxy', function() {
  const outDir = 'build';
  execSync(`rm -rf ${outDir}`);
  execSync(`cp -r ${STATIC}/ ${outDir}/`);
  copyTranslations(outDir);
  concatJS(outDir, 'badge', 'embed.js', availableLangs());
  writeFileSync(`${outDir}/index.js`, translatedLangs(), 'utf8');
  execSync(`cat ${STATIC}/index.js >> ${outDir}/index.js`);
  console.log('Snowflake prepared.');
});

task('webext', 'build the webextension', function() {
  const outDir = 'webext';
  execSync(`git clean -f -x -d ${outDir}/`);
  execSync(`cp -r ${STATIC}/{${SHARED_FILES.join(',')}} ${outDir}/`, { shell: '/bin/bash' });
  copyTranslations(outDir);
  concatJS(outDir, 'webext', 'snowflake.js', '');
  console.log('Webextension prepared.');
});

task('node', 'build the node binary', function() {
  execSync('mkdir -p build');
  concatJS('build', 'node', 'snowflake.js', '');
  console.log('Node prepared.');
});

task('pack-webext', 'pack the webextension for deployment', function() {
  try {
    execSync(`rm -f source.zip`);
    execSync(`rm -f webext/webext.zip`);
  } catch (error) {
    //Usually this happens because the zip files were removed previously
    console.log('Error removing zip files');
  }
  execSync(`git submodule update --remote`);
  var version = process.argv[3];
  console.log(version);
  var manifest = require('./webext/manifest.json')
  manifest.version = version;
  writeFileSync('./webext/manifest.json', JSON.stringify(manifest, null, 2), 'utf8');
  execSync(`git commit -am "bump version to ${version}"`);
  try {
    execSync(`git tag webext-${version}`);
  } catch (error) {
    console.log('Error creating git tag');
    // Revert changes
    execSync(`git reset HEAD~`);
    execSync(`git checkout ./webext/manifest.json`);
    execSync(`git submodule update`);
    return;
  }
  execSync(`git archive -o source.zip HEAD .`);
  execSync(`npm run webext`);
  execSync(`cd webext && zip -Xr webext.zip ./*`);
});

task('clean', 'remove all built files', function() {
  execSync('rm -rf build test spec/support');
});

task('library', 'build the library', function() {
  concatJS('.', '', 'snowflake-library.js', '');
  console.log('Library prepared.');
});

var cmd = process.argv[2];

if (tasks.has(cmd)) {
  var t = tasks.get(cmd);
  console.log(t.msg);
  t.func();
} else {
  console.error('Command not supported.');

  console.log('Commands:');

  tasks.forEach(function(value, key) {
    console.log(key + ' - ' + value.msg);
  })
}
