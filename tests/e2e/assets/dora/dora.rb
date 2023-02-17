ENV['RACK_ENV'] ||= 'development'

require 'rubygems'
require 'sinatra/base'
require 'json'

ID = ((ENV["VCAP_APPLICATION"] && JSON.parse(ENV["VCAP_APPLICATION"])["instance_id"]) || SecureRandom.uuid).freeze

require "instances"
require "stress_testers"
require "log_utils"
require "curl"
require 'bundler'
Bundler.require :default, ENV['RACK_ENV'].to_sym

$stdout.sync = true
$stderr.sync = true
$counter = 0
$start_time = Time.now

class Dora < Sinatra::Base
  use Instances
  use StressTesters
  use LogUtils
  use Curl

  get '/' do
    "Hi, I'm Dora!"
  end

  get '/health' do
    $stderr.puts("Called /health #{$counter}")
    if $counter < 3
      $counter += 1
     status 500
     body "Hit /health #{$counter} times"
    else
      "I'm alive"
    end
  end

  get '/ping/:address' do
    `ping -c 4 #{params[:address]}`
  end

  get '/lsb_release' do
    `lsb_release --all`
  end

  get '/find/:filename' do
    `find / -name #{params[:filename]}`
  end

  get '/sigterm' do
    "Available sigterms #{Signal.list.keys}"
  end

  get '/dpkg/:package' do
    puts "Sending dpkg output for #{params[:package]}"
    `dpkg -l #{params[:package]}`
  end

  get '/delay/:seconds' do
    sleep params[:seconds].to_i
    "YAWN! Slept so well for #{params[:seconds].to_i} seconds"
  end

  get '/sigterm/:signal' do
    pid = Process.pid
    signal = params[:signal]
    puts "Killing process #{pid} with signal #{signal}"
    Process.kill(signal, pid)
  end

  get '/logspew/:kbytes' do
    kb = "1" * 1024 ;
    params[:kbytes].to_i.times { puts kb }
    "Just wrote #{params[:kbytes]} kbytes to the log"
  end

  get '/echo/:destination/:output' do
    redirect =
        case params[:destination]
          when "stdout"
            ""
          when "stderr"
            " 1>&2"
          else
            " > #{params[:destination]}"
        end

    system "echo '#{params[:output]}'#{redirect}"

    "Printed '#{params[:output]}' to #{params[:destination]}!"
  end

  get '/env/:name' do
    ENV[params[:name]]
  end

  get '/env' do
    ENV.to_hash.to_s
  end

  get '/env.json' do
    ENV.to_hash.to_json
  end

  get '/myip' do
    `ip route get 1 | awk '{print $NF;exit}'`
  end

  get '/largetext/:kbytes' do
    fiveMB = 5 * 1024
    numKB = params[:kbytes].to_i
    ktext="1" * 1024
    text=""
    size = numKB > fiveMB ? fiveMB : numKB
    size.times {text+=ktext}
    text
  end

  get '/uptime' do
    (Time.now - $start_time).to_s
  end

  run! if app_file == $0
end
