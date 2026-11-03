# 08 Families
Route: /admin/families. Screenshots: screenshots/08-families/.
- Households grouped by a **family number (F-00001...)**, auto-created when siblings share the same father mobile during enrollment. Tiles: families (98), students linked (206), parent accounts linked (98 = one login per family).
- List: search by family number, label, student or parent name; CSV/PDF; columns family #, label (optional name), students count, parents count, invoices (open invoice count), status Active, view/edit/delete.
- One shared parent-app login per family; username is the father's mobile. Siblings drive automatic "SIBLING" discounts at student level.
- Entities: Family(id, number, label, status), ParentAccount(family_id, mobile), Guardian(family_id, relation, name, mobile, income), link Student.family_id.
- Gaps/ideas: 2 students are not linked to any family (206 of 208) so data hygiene matters; family-level consolidated fee statement and single payment across siblings is a strong parent value; support mother/guardian as separate logins with roles; merge duplicate families tool.
