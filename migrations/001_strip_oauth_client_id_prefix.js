// Migration: Strip fc_ prefix from OAuth client IDs
// Run this in MongoDB shell: mongosh flowcatalyst < 001_strip_oauth_client_id_prefix.js
// Or in mongosh: load("001_strip_oauth_client_id_prefix.js")

print("Starting migration: Strip fc_ prefix from OAuth client IDs");

// Migrate oauth_clients collection
var clientCount = 0;
db.oauth_clients.find({ clientId: /^fc_/ }).forEach(function(doc) {
  var newClientId = doc.clientId.substring(3); // Remove "fc_" prefix
  print("  oauth_clients: " + doc.clientId + " -> " + newClientId);
  db.oauth_clients.updateOne(
    { _id: doc._id },
    { $set: { clientId: newClientId } }
  );
  clientCount++;
});
print("Migrated " + clientCount + " oauth_clients");

// Migrate authorization_codes collection
var codeCount = 0;
db.authorization_codes.find({ clientId: /^fc_/ }).forEach(function(doc) {
  var newClientId = doc.clientId.substring(3);
  db.authorization_codes.updateOne(
    { _id: doc._id },
    { $set: { clientId: newClientId } }
  );
  codeCount++;
});
print("Migrated " + codeCount + " authorization_codes");

// Migrate refresh_tokens collection
var tokenCount = 0;
db.refresh_tokens.find({ clientId: /^fc_/ }).forEach(function(doc) {
  var newClientId = doc.clientId.substring(3);
  db.refresh_tokens.updateOne(
    { _id: doc._id },
    { $set: { clientId: newClientId } }
  );
  tokenCount++;
});
print("Migrated " + tokenCount + " refresh_tokens");

print("Migration complete: " + (clientCount + codeCount + tokenCount) + " total documents updated");
