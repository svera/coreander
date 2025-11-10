"use strict";

// Handle checkbox change using plain fetch (not htmx to avoid event bubbling issues)
document.body.addEventListener('change', function(evt) {
    if (!evt.target.id || !evt.target.id.startsWith('complete-checkbox-')) {
        return;
    }

    const checkboxEl = evt.target;
    const slug = checkboxEl.getAttribute('data-slug');

    // Send POST request to toggle completion
    fetch(`/documents/${slug}/complete`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        credentials: 'same-origin'
    })
    .then(response => {
        if (response.status === 403) {
            // Session expired
            window.location.reload();
            return;
        }

        if (!response.ok) {
            throw new Error('Request failed');
        }

        // Update the UI
        const dateContainer = document.getElementById(`completion-date-${slug}`);
        const labelEl = document.getElementById(`complete-label-${slug}`);
        const dateInput = dateContainer?.querySelector('input[type="date"]');

        if (checkboxEl.checked) {
            checkboxEl.title = checkboxEl.getAttribute('data-incomplete-title');
            if (labelEl) {
                labelEl.textContent = checkboxEl.getAttribute('data-completed-label');
            }

            // Show and enable date picker
            if (dateInput) {
                const today = new Date().toISOString().split('T')[0];
                dateInput.value = today;
                dateInput.setAttribute('data-original-date', today);
                dateInput.classList.remove('d-none');
                dateInput.disabled = false;
            }
        } else {
            checkboxEl.title = checkboxEl.getAttribute('data-complete-title');
            if (labelEl) {
                labelEl.textContent = checkboxEl.getAttribute('data-uncompleted-label');
            }

            // Hide and disable date picker
            if (dateInput) {
                dateInput.value = '';
                dateInput.setAttribute('data-original-date', '');
                dateInput.classList.add('d-none');
                dateInput.disabled = true;
            }
        }
    })
    .catch(error => {
        console.error('Error toggling completion status:', error);
        // Revert checkbox state on error
        checkboxEl.checked = !checkboxEl.checked;
    });
});

// Function to initialize date inputs
function initializeDateInputs() {
    const today = new Date().toISOString().split('T')[0];
    document.querySelectorAll('[id^="completion-date-"] input[type="date"]').forEach(input => {
        if (!input.hasAttribute('data-initialized')) {
            input.setAttribute('max', today);
            input.setAttribute('data-initialized', 'true');
        }
    });
}

// Handle htmx content loading
document.body.addEventListener('htmx:afterSwap', function() {
    initializeDateInputs();
});

// Handle completion date changes
document.addEventListener('DOMContentLoaded', function() {
    // Set max date to today for all completion date inputs
    initializeDateInputs();

    document.body.addEventListener('change', function(evt) {
        // Check if this is a date input within a completion-date container
        if (evt.target.type !== 'date' || !evt.target.dataset.slug) {
            return;
        }

        // Check if parent has completion-date ID
        const container = evt.target.parentElement;
        if (!container || !container.id || !container.id.startsWith('completion-date-')) {
            return;
        }

        const input = evt.target;
        const slug = input.dataset.slug;
        const newDate = input.value;
        const originalDate = input.dataset.originalDate;

        // Only update if the date actually changed
        if (newDate === originalDate) {
            return;
        }

        // Validate date format
        if (!newDate || !/^\d{4}-\d{2}-\d{2}$/.test(newDate)) {
            console.error('Invalid date format:', newDate);
            input.value = originalDate;
            return;
        }

        // Prevent future dates (compare only date parts)
        const selectedDate = new Date(newDate + 'T00:00:00');
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        if (selectedDate > today) {
            console.error('Future dates not allowed:', newDate);
            input.value = originalDate;
            return;
        }

        // Send the update request
        fetch(`/documents/${slug}/complete`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
            },
            credentials: 'same-origin',
            body: JSON.stringify({ completed_at: newDate })
        })
        .then(response => {
            if (response.ok) {
                // Update the original date to the new value
                input.dataset.originalDate = newDate;
            } else if (response.status === 403) {
                // Session expired, reload the page
                window.location.reload();
            } else {
                throw new Error('Failed to update date');
            }
        })
        .catch(error => {
            console.error('Error updating completion date:', error);
            // Revert to original date silently
            input.value = originalDate;
        });
    });
});


