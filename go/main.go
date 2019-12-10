package main

import (
        "context"
        "encoding/base64"
        "fmt"
        "io/ioutil"
        "log"
        "go.mongodb.org/mongo-driver/bson"
        "go.mongodb.org/mongo-driver/mongo"
        "go.mongodb.org/mongo-driver/mongo/options"
)

const (
        uri = "mongodb://localhost:27017"
)

var (
        ctx = context.Background()
        // Test key material generated by: echo $(head -c 96 /dev/urandom | base64 | tr -d '\n')
        localMasterKey = "E7h/7bm+gvHPosEhZLB96Nms4Lrn2zV90vKpVJIo7UMn9112iex7dPeHHKVt088kNr3Zv+ZpBGZTYFI7yVm49eIysA7PwXdZ/QpNcwuw9Ut5rYiXXf4UA8G9fNNkYonQ"
        kmsProviders   map[string]map[string]interface{}
)

func main() {
        // initial setup
        decodedKey, err := base64.StdEncoding.DecodeString(localMasterKey)
        if err != nil {
                log.Fatalf("base64 decode error: %v", err)
        }
        kmsProviders = map[string]map[string]interface{}{
                "local": {"key": decodedKey},
        }

        createDataKey()
        client := createEncryptedClient()
        defer client.Disconnect(ctx)

        coll := client.Database("foo").Collection("bar")
        _ = coll.Drop(ctx)

        // insert a document with an encrypted field and a plaintext field
        _, err = coll.InsertOne(ctx, bson.M{
                "plaintext":       "hello world",
                "encrypted_field": "encrypted",
                "altname":         "altname",
        })
        if err != nil {
                log.Fatalf("InsertOne error: %v", err)
        }

        // find and print the inserted document
        res, err := coll.FindOne(ctx, bson.D{}).DecodeBytes()
        if err != nil {
                log.Fatalf("FindOne error: %v", err)
        }
        fmt.Println(res)
}

// create a new data key with an alternate key name
func createDataKey() {
        // create key vault client and drop key vault collection
        kvClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
        if err != nil {
                log.Fatalf("Connect error for key vault client: %v", err)
        }
        _ = kvClient.Database("keyvault").Collection("__datakeys").Drop(ctx)

        // create ClientEncryption
        clientEncOpts := options.ClientEncryption().SetKeyVaultNamespace("keyvault.__datakeys").SetKmsProviders(kmsProviders)
        clientEnc, err := mongo.NewClientEncryption(kvClient, clientEncOpts)
        if err != nil {
                log.Fatalf("NewClientEncryption error: %v", err)
        }
        defer clientEnc.Close(ctx)

        // create a new data key
        dataKeyOpts := options.DataKey().SetKeyAltNames([]string{"altname"})
        _, err = clientEnc.CreateDataKey(ctx, "local", dataKeyOpts)
        if err != nil {
                log.Fatalf("CreateDataKey error: %v", err)
        }
}

// create a client configured with auto encryption that uses the key generated by createDataKey
func createEncryptedClient() *mongo.Client {
        // create a client with auto encryption
        schemaMap := map[string]interface{}{
                "foo.bar": readJSONFile("collection_schema.json"),
        }
        autoEncOpts := options.AutoEncryption().
                SetKeyVaultNamespace("keyvault.__datakeys").
                SetKmsProviders(kmsProviders).
                SetSchemaMap(schemaMap)

        clientOpts := options.Client().ApplyURI(uri).SetAutoEncryptionOptions(autoEncOpts)
        autoEncryptionClient, err := mongo.Connect(ctx, clientOpts)
        if err != nil {
                log.Fatalf("Connect error for client with automatic encryption: %v", err)
        }
        return autoEncryptionClient
}

func readJSONFile(file string) bson.D {
        content, err := ioutil.ReadFile(file)
        if err != nil {
                log.Fatalf("ReadFile error for %v: %v", file, err)
        }

        var fileDoc bson.D
        if err = bson.UnmarshalExtJSON(content, false, &fileDoc); err != nil {
                log.Fatalf("UnmarshalExtJSON error for file %v: %v", file, err)
        }
        return fileDoc
}