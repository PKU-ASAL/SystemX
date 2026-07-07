# frozen_string_literal: true

begin
  require "fog/libvirt"

  unless Fog::Libvirt::Compute.recognized.include?(:libvirt_ip_command)
    Fog::Libvirt::Compute.recognizes(:libvirt_ip_command)
  end
rescue LoadError, NameError
  # The libvirt provider loads this dependency only when available.
end
