<?php

namespace FlowCatalyst\Generated\Endpoint;

class ConfigureApplicationForClient extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $clientId;
    protected $id;
    /**
     * @param string $clientId
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ClientConfigRequest $requestBody
     */
    public function __construct(string $clientId, string $id, ?\FlowCatalyst\Generated\Model\ClientConfigRequest $requestBody = null)
    {
        $this->clientId = $clientId;
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{clientId}', '{id}'], [$this->clientId, $this->id], '/api/admin/applications/{id}/clients/{clientId}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ClientConfigRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\ConfigureApplicationForClientBadRequestException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\ConfigureApplicationForClientBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}