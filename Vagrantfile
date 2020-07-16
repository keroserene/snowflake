require 'pathname'
require 'tempfile'
require 'yaml'

srvpath = Pathname.new(File.dirname(__FILE__)).realpath
configfile = YAML.load_file(File.join(srvpath, "/.gitlab-ci.yml"))
remote_url = 'https://git.torproject.org/pluggable-transports/snowflake.git'

# set up essential environment variables
env = configfile['variables']
env = env.merge(configfile['android']['variables'])
env['CI_PROJECT_DIR'] = '/builds/tpo/anti-censorship/pluggable-transports/snowflake'
env_file = Tempfile.new('env')
File.chmod(0644, env_file.path)
env.each do |k,v|
    env_file.write("export #{k}='#{v}'\n")
end
env_file.rewind

sourcepath = '/etc/profile.d/env.sh'
header = "#!/bin/bash -ex\nsource #{sourcepath}\ncd $CI_PROJECT_DIR\n"

before_script_file = Tempfile.new('before_script')
File.chmod(0755, before_script_file.path)
before_script_file.write(header)
configfile['android']['before_script'].flatten.each do |line|
    before_script_file.write(line)
    before_script_file.write("\n")
end
before_script_file.rewind

script_file = Tempfile.new('script')
File.chmod(0755, script_file.path)
script_file.write(header)
configfile['android']['script'].flatten.each do |line|
    script_file.write(line)
    script_file.write("\n")
end
script_file.rewind

Vagrant.configure("2") do |config|
  config.vm.box = "debian/bullseye64"
  config.vm.synced_folder '.', '/vagrant', disabled: true
  config.vm.provision "file", source: env_file.path, destination: 'env.sh'
  config.vm.provision :shell, inline: <<-SHELL
    set -ex
    mv ~vagrant/env.sh #{sourcepath}
    source #{sourcepath}
    test -d /go || mkdir /go
    mkdir -p $(dirname $CI_PROJECT_DIR)
    chown -R vagrant.vagrant $(dirname $CI_PROJECT_DIR)
    apt-get update
    apt-get -qy install --no-install-recommends git
    git clone #{remote_url} $CI_PROJECT_DIR
    chmod -R a+rX,u+w /go $CI_PROJECT_DIR
    chown -R vagrant.vagrant /go $CI_PROJECT_DIR
SHELL
  config.vm.provision "file", source: before_script_file.path, destination: 'before_script.sh'
  config.vm.provision "file", source: script_file.path, destination: 'script.sh'
  config.vm.provision :shell, inline: '/home/vagrant/before_script.sh'
  config.vm.provision :shell, privileged: false, inline: '/home/vagrant/script.sh'

  # remove this or comment it out to use VirtualBox instead of libvirt
  config.vm.provider :libvirt do |libvirt|
    libvirt.memory = 1536
  end
end
