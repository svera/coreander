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
        const dateContainer = document.getElementById(`completion-date-dd-${slug}`);
        const labelEl = document.getElementById(`complete-label-${slug}`);

        if (checkboxEl.checked) {
            checkboxEl.title = checkboxEl.getAttribute('data-incomplete-title');
            if (labelEl) {
                labelEl.textContent = checkboxEl.getAttribute('data-completed-label');
            }

            // Add date picker if checked
            if (dateContainer && !dateContainer.querySelector('input[type="date"]')) {
                const today = new Date().toISOString().split('T')[0];
                const dateInput = document.createElement('input');
                dateInput.type = 'date';

                // Check if container is a dd element (document-metadata) or span (docs-list)
                if (dateContainer.tagName.toLowerCase() === 'dd') {
                    dateInput.className = 'border-0 border-bottom text-end text-muted bg-transparent p-0';
                } else {
                    dateInput.className = 'border-0 border-bottom text-muted bg-transparent p-0 ms-1';
                }

                dateInput.id = `completion-date-${slug}`;
                dateInput.value = today;
                dateInput.setAttribute('data-slug', slug);
                dateInput.setAttribute('data-original-date', today);
                dateInput.setAttribute('max', today);
                dateInput.setAttribute('title', 'Edit completion date');
                dateContainer.appendChild(dateInput);
            }
        } else {
            checkboxEl.title = checkboxEl.getAttribute('data-complete-title');
            if (labelEl) {
                labelEl.textContent = checkboxEl.getAttribute('data-uncompleted-label');
            }

            // Remove date picker if unchecked
            if (dateContainer) {
                dateContainer.innerHTML = '';
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
    document.querySelectorAll('input[id^="completion-date-"]').forEach(input => {
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
        if (!evt.target.id || !evt.target.id.startsWith("completion-date-") || evt.target.id.startsWith("completion-date-dd-")) {
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


