import { jsPDF } from "jspdf";
import autoTable from "jspdf-autotable";
import html2canvas from "html2canvas";

const STATUS_LABELS = { todo: "To Do", in_progress: "In Progress", review: "Review", done: "Done" };

const fmtHours = (mins) => (mins / 60).toFixed(1);

function buildSummaryRows(data) {
  return [
    ["Total Tasks", String(data.total_tasks)],
    ["Completed", String(data.completed_tasks)],
    ["Completion Rate", `${data.completion_rate.toFixed(0)}%`],
    ["Hours Logged", fmtHours(data.total_logged_minutes)],
    ["Hours Estimated", fmtHours(data.total_estimate_minutes)],
  ];
}

function buildStatusRows(data) {
  return Object.entries(data.by_status).map(([k, v]) => [STATUS_LABELS[k] || k, String(v)]);
}

function buildDailyRows(data) {
  return data.time_series.map((t, i) => [t.date, String(t.minutes), String(data.completed_series[i]?.completed || 0)]);
}

function buildTaskRows({ tasks, projectMap, memberMap }) {
  return tasks.map((t) => {
    const proj = projectMap[t.project_id];
    const a = memberMap[t.assignee_id];
    return [
      t.title,
      proj ? proj.name : "—",
      STATUS_LABELS[t.status] || t.status,
      a ? (a.name || a.email) : "Unassigned",
      fmtHours(t.logged_minutes || 0),
    ];
  });
}

function csvEscape(v) {
  const s = String(v ?? "");
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

function toCsvSection(title, header, rows) {
  const lines = [title, header.map(csvEscape).join(",")];
  rows.forEach((r) => lines.push(r.map(csvEscape).join(",")));
  return lines.join("\n");
}

export function exportCSV({ orgName, range, data, tasks, projectMap, memberMap }) {
  const parts = [
    `Analytics Report,${csvEscape(orgName)}`,
    `Period,${range.start} to ${range.end}`,
    "",
    toCsvSection("Summary", ["Metric", "Value"], buildSummaryRows(data)),
    "",
    toCsvSection("Status Breakdown", ["Status", "Count"], buildStatusRows(data)),
    "",
    toCsvSection("Daily", ["Date", "Minutes Logged", "Tasks Completed"], buildDailyRows(data)),
    "",
    toCsvSection("Tasks", ["Title", "Project", "Status", "Assignee", "Hours Logged"], buildTaskRows({ tasks, projectMap, memberMap })),
  ];
  const blob = new Blob([parts.join("\n")], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `analytics_${range.start}_${range.end}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

export async function exportPDF({ orgName, range, data, tasks, projectMap, memberMap, chartsEl }) {
  const doc = new jsPDF({ unit: "pt", format: "a4" });
  const pageW = doc.internal.pageSize.getWidth();
  const margin = 40;
  let y = margin;

  doc.setFontSize(18);
  doc.setTextColor(30, 41, 59);
  doc.text("Analytics Report", margin, y);
  y += 20;
  doc.setFontSize(11);
  doc.setTextColor(100, 116, 139);
  doc.text(orgName, margin, y);
  y += 15;
  doc.text(`Period: ${range.start}  to  ${range.end}`, margin, y);
  y += 18;

  autoTable(doc, {
    startY: y,
    head: [["Metric", "Value"]],
    body: buildSummaryRows(data),
    theme: "grid",
    headStyles: { fillColor: [79, 70, 229] },
    styles: { fontSize: 9 },
    margin: { left: margin, right: margin },
  });
  y = doc.lastAutoTable.finalY + 20;

  if (chartsEl) {
    try {
      const canvas = await html2canvas(chartsEl, { scale: 2, backgroundColor: "#ffffff", useCORS: true });
      const imgW = pageW - margin * 2;
      const imgH = (canvas.height * imgW) / canvas.width;
      if (y + imgH > doc.internal.pageSize.getHeight() - margin) {
        doc.addPage();
        y = margin;
      }
      doc.addImage(canvas.toDataURL("image/png"), "PNG", margin, y, imgW, imgH);
      y += imgH + 20;
    } catch (e) {
      // charts optional — continue without image
    }
  }

  autoTable(doc, {
    startY: y,
    head: [["Status", "Count"]],
    body: buildStatusRows(data),
    theme: "grid",
    headStyles: { fillColor: [79, 70, 229] },
    styles: { fontSize: 9 },
    margin: { left: margin, right: margin },
  });
  y = doc.lastAutoTable.finalY + 20;

  autoTable(doc, {
    startY: y,
    head: [["Date", "Minutes Logged", "Tasks Completed"]],
    body: buildDailyRows(data),
    theme: "striped",
    headStyles: { fillColor: [79, 70, 229] },
    styles: { fontSize: 8 },
    margin: { left: margin, right: margin },
  });
  y = doc.lastAutoTable.finalY + 20;

  autoTable(doc, {
    startY: y,
    head: [["Title", "Project", "Status", "Assignee", "Hours"]],
    body: buildTaskRows({ tasks, projectMap, memberMap }),
    theme: "striped",
    headStyles: { fillColor: [79, 70, 229] },
    styles: { fontSize: 8, cellPadding: 3 },
    columnStyles: { 0: { cellWidth: 180 } },
    margin: { left: margin, right: margin },
  });

  doc.save(`analytics_${range.start}_${range.end}.pdf`);
}
