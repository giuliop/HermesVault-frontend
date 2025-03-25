const Trim = {
    /**
     * Trim the value of the input element to a maximum length and display a
     * trimmed version with ellipsis. Save the full value in the data-full-value
     * attribute
     */
    trim: function (elem, maxLen = 10) {
        const halfLen = Math.floor(maxLen / 2);
        const fullValue = elem.value;
        elem.dataset.fullValue = fullValue;
        if (fullValue.length > maxLen) {
            elem.value = fullValue.slice(0, halfLen)
                + '...'
                + fullValue.slice(-halfLen);
        }
    },

    /**
     * Restore the full value of the input element from the data-full-value
     * attribute (if it exists)
     */
    restore: function (elem) {
        elem.value = elem.dataset.fullValue || elem.value;
    },

    /**
     * Restore the full value of all input elements in the form from the
     * data-full-value attribute (if it exists)
     */
    restoreAll: function (event) {
        let form = event.target;
        let formData = event.detail.parameters;
        for (let input of form.elements) {
            if (input.dataset.fullValue) {
                formData[input.name] = input.dataset.fullValue;
            }
        }
    },
}

const Show = {
    /**
     * Show a tooltip for `elem` with a `message` that fades out after `duration`
     */
    fadingTooltip: function (elem, message, duration = 1000) {
        let tooltip = document.createElement('div');

        tooltip.classList.add('tooltip');
        tooltip.textContent = message;

        let boundingBox = elem.getBoundingClientRect();
        tooltip.style.position = 'absolute';
        tooltip.style.left = `${boundingBox.left + window.scrollX}px`;
        tooltip.style.top = `${boundingBox.top + window.scrollY}px`;
        tooltip.style.display = 'block';

        document.body.appendChild(tooltip);
        setTimeout(() => {
            document.body.removeChild(tooltip);
        }, duration);;
    },

    /**
     * Scroll to the element defined by `selector`
     * Set `afterEvent` to false to scroll before the potential triggering event
     */
    scrollTo: function(selector, afterEvent = true, scrollType = 'smooth') {
        let elem = document.querySelector(selector);
        let f = () => {
            let elementBottom = elem.offsetTop + elem.offsetHeight;
            window.scrollTo({ top: elementBottom - window.innerHeight / 2,
                              behavior: scrollType})
        }
        if (afterEvent) {
            setTimeout(f, 0);
        }
        else {
            f();
        }
    },

    /**
     * Show the email address in the supplied element ID
     */
    email: function (elemId) {
        // Email obfuscation with JavaScript and character encoding
        window.addEventListener('DOMContentLoaded', function () {
            const displayEmail = function () {
                // Email components for info@...
                const username = String.fromCharCode(105, 110, 102, 111); // info
                const separator = String.fromCharCode(64);
                const domain = String.fromCharCode(104, 101, 114, 109, 101, 115, 118, 97, 117, 108, 116);
                const dot = String.fromCharCode(46);
                const tld = String.fromCharCode(111, 114, 103);

                const email = username + separator + domain + dot + tld;
                const html = "<a href='mailto:" + email + "'>" + email + "</a>";

                // Assemble the email with a slight delay to further confuse bots
                setTimeout(function () {
                    document.getElementById(elemId).innerHTML = html
                }, 100);
            };

            // Call the function on page load but with a small delay
            setTimeout(displayEmail, 100);
        });;
    }
}

const Style = {
    toggleDarkMode: function() {
        if(document.documentElement.classList.contains("-no-dark-theme")){
            // Enable dark mode
            document.documentElement.classList.remove("-no-dark-theme");
            document.documentElement.classList.add("dark-theme");
            return;
        }
        // Enable light mode
        document.documentElement.classList.add("-no-dark-theme");
        document.documentElement.classList.remove("dark-theme");
    }
}

const Form = {
    /**
     * Disable a submit button after click to prevent multiple submissions
     * The button will be re-enabled when the response comes back (useful if errors come back)
     * or after 1 minute as a fallback
     */
    disableSubmitButton: function (event) {
        const button = event.submitter;
        if (button && button.type === 'submit') {
            button.disabled = true;
            button.dataset.originalText = button.innerText;
            button.innerText = 'Processing...';

            function restoreButton() {
                button.disabled = false;
                if (button.dataset.originalText) {
                    button.innerText = button.dataset.originalText;
                }
                document.removeEventListener('htmx:afterRequest', reEnable);
            }

            function reEnable(e) {
                if (e.detail.elt === event.target) {
                    restoreButton();
                    clearTimeout(timeoutId);
                }
            }

            // Create timeout to re-enable after 1 minute as a fallback
            // This will also avoid memory leaks if the user navigates away
            const timeoutId = setTimeout(restoreButton, 60000);

            // Re-enable the button when the request is complete
            document.addEventListener('htmx:afterRequest', reEnable);
        }
    }
}

const History = {
    /**
     * Add the path to the history
     */
    add: function (path) {
        window.history.pushState({ path: path }, '', window.location.href);
        // console.log('Added path to history:', path);
    },

    /**
     * Load the path from the history into the target element
     * Typically triggered by the `popstate` event
     */
    load: function (event, target, defaultPath) {
        let path = (event.state && event.state.path) || defaultPath;
        if (target && path) {
            // console.log("Loading path from history:", path);
            htmx.ajax('GET', path, {
                target: target,
                swap: target.getAttribute('hx-swap') || 'innerHTML',
            });;
        }
    }
}

const behaviors = {
    Trim: Trim,
    Show: Show,
    Style: Style,
    Form: Form,
    History: History,
};

window.behaviors = behaviors;