"use strict";

import { DateTime } from "./luxon.min.js";

document.addEventListener('DOMContentLoaded', function() {
    const datetime = document.querySelectorAll('.datetime span');
    datetime.forEach(function(element) {
        const dt = DateTime.fromISO(element.textContent);
        element.textContent = dt.toRelative({ locale: document.documentElement.lang });
    });
});