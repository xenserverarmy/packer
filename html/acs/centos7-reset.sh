# remove device rules
rm -f /etc/udev/rules.d/70*
rm -f /var/lib/dhcp/dhclient.*

# remove/clear generic log files
cat /dev/null > /var/log/audit/audit.log 2>/dev/null
cat /dev/null > /var/log/wtmp 2>/dev/null
cat /dev/null > /var/log/messages 2>/dev/null
logrotate -f /etc/logrotate.conf 2>/dev/null
rm -f /var/log/*-* /var/log/*.gz 2>/dev/null

#reset the hostname so we get it automatically upon template provisioning
hostname localhost
echo "localhost" > /etc/hostname

#clear history
cat /dev/null > ~/.bash_history && history -c && unset HISTFILE
