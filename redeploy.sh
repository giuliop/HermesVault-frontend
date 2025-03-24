#!/bin/bash
set -euo pipefail

# When an error occurs, run the ERR trap which logs a message.
trap 'echo "An error occurred. Please check the logs. Maintenance mode remains enabled."' ERR

echo "Enabling maintenance mode..."
sudo touch /var/www/hermesvault/maintenance.enable

echo "Waiting 2 seconds for Apache to pick up maintenance mode..."
sleep 2

echo "Building frontend assets..."
npm run build --prefix frontend

echo "Building Go binary..."
go build -o ./.tmp/main .

# Function to restart a service with error handling and informative output.
restart_service() {
    local service_name="$1"
    echo "Restarting ${service_name}..."
    if ! sudo systemctl restart "$service_name"; then
        echo "Error restarting ${service_name} service:"
        sudo systemctl status "$service_name"
        exit 1
    fi
    echo "${service_name} restarted successfully."
}

echo "Restarting services..."
restart_service hermesvault-frontend-go-webserver
restart_service hermesvault-frontend-python-subscriber

echo "Waiting 5 seconds for services to come online..."
sleep 5

echo "Disabling maintenance mode..."
sudo rm /var/www/hermesvault/maintenance.enable

echo "Deployment completed successfully."
