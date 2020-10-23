package topicconfig

import (
	"errors"
	"github.com/segmentio/kafka-go"
	"github.com/wvanbergen/kazoo-go"
	"io/ioutil"
	"log"
)

func Create(zkUrl string, topic string, partitions int, replicationfactor int, config map[string]string) (err error) {
	controller, err := GetKafkaController(zkUrl)
	if err != nil {
		log.Println("ERROR: unable to find controller", err)
		return err
	}
	if controller == "" {
		log.Println("ERROR: unable to find controller")
		return errors.New("unable to find controller")
	}
	initConn, err := kafka.Dial("tcp", controller)
	if err != nil {
		log.Println("ERROR: while init topic connection ", err)
		return err
	}

	entries := []kafka.ConfigEntry{}
	for key, value := range config {
		entries = append(entries, kafka.ConfigEntry{
			ConfigName:  key,
			ConfigValue: value,
		})
	}

	defer initConn.Close()
	err = initConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: replicationfactor,
		ConfigEntries:     entries,
	})
	if err != nil {
		return
	}
	return nil
}

func GetKafkaController(zkUrl string) (controller string, err error) {
	zookeeper := kazoo.NewConfig()
	zookeeper.Logger = log.New(ioutil.Discard, "", 0)
	zk, chroot := kazoo.ParseConnectionString(zkUrl)
	zookeeper.Chroot = chroot
	kz, err := kazoo.NewKazoo(zk, zookeeper)
	if err != nil {
		return controller, err
	}
	controllerId, err := kz.Controller()
	if err != nil {
		return controller, err
	}
	brokers, err := kz.Brokers()
	kz.Close()
	if err != nil {
		return controller, err
	}
	return brokers[controllerId], err
}
