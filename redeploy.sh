#!/bin/bash
set -e  # exit on any error

# When an error occurs, run the ERR trap which logs a message.
trap 'echo "An error occurred. Please check the logs. Maintenance mode remains enabled."' ERR

# Enable maintenance mode so that Apache serves the maintenance page
sudo touch /var/www/hermesvault/maintenance.enable

# Allow Apache time to pick up the change
sleep 2

# Build the frontend assets
npm run build --prefix frontend

# Build the Go binary
go build -o ./.tmp/main .

# Restart the Go webserver service
sudo systemctl restart hermesvault-frontend-go-webserver
# Restart the subscriber service
sudo systemctl restart hermesvault-frontend-python-subscriber

# Allow time for the service to come online
sleep 5

# Disable maintenance mode to resume normal operations
sudo rm /var/www/hermesvault/maintenance.enable
