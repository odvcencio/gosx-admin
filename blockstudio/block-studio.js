(function () {
  function init(list) {
    var dragging = null;

    function rows() {
      return Array.prototype.slice.call(list.querySelectorAll("[data-block-studio-block]"));
    }

    function orderInput(row) {
      return row.querySelector("[data-block-studio-order]");
    }

    function renumber() {
      var current = rows();
      current.forEach(function (row, index) {
        var input = orderInput(row);
        if (input) {
          input.value = String(index + 1);
          input.max = String(current.length);
        }
        var up = row.querySelector('[data-block-studio-move="up"]');
        var down = row.querySelector('[data-block-studio-move="down"]');
        if (up) {
          up.disabled = index === 0;
        }
        if (down) {
          down.disabled = index === current.length - 1;
        }
      });
    }

    function move(row, direction) {
      if (!row) {
        return;
      }
      if (direction === "up" && row.previousElementSibling) {
        list.insertBefore(row, row.previousElementSibling);
      }
      if (direction === "down" && row.nextElementSibling) {
        list.insertBefore(row.nextElementSibling, row);
      }
      renumber();
      row.focus({ preventScroll: true });
    }

    list.addEventListener("click", function (event) {
      var button = event.target.closest("[data-block-studio-move]");
      if (!button || !list.contains(button)) {
        return;
      }
      move(button.closest("[data-block-studio-block]"), button.getAttribute("data-block-studio-move"));
    });

    list.addEventListener("dragstart", function (event) {
      var handle = event.target.closest("[data-block-studio-handle]");
      var row = event.target.closest("[data-block-studio-block]");
      if (!handle || !row || !list.contains(row)) {
        event.preventDefault();
        return;
      }
      dragging = row;
      row.classList.add("is-dragging");
      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", row.getAttribute("data-block-studio-block") || "");
      }
    });

    list.addEventListener("dragover", function (event) {
      if (!dragging) {
        return;
      }
      event.preventDefault();
      var target = event.target.closest("[data-block-studio-block]");
      if (!target || target === dragging || !list.contains(target)) {
        return;
      }
      var bounds = target.getBoundingClientRect();
      var after = event.clientY > bounds.top + bounds.height / 2;
      list.insertBefore(dragging, after ? target.nextSibling : target);
    });

    list.addEventListener("drop", function (event) {
      if (!dragging) {
        return;
      }
      event.preventDefault();
      dragging.classList.remove("is-dragging");
      dragging = null;
      renumber();
    });

    list.addEventListener("dragend", function () {
      if (dragging) {
        dragging.classList.remove("is-dragging");
        dragging = null;
      }
      renumber();
    });

    renumber();
  }

  Array.prototype.forEach.call(document.querySelectorAll("[data-block-studio]"), init);
})();
